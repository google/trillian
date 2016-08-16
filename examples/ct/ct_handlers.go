package ct

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
)

const (
	// All RFC6962 requests start with this base path
	ctV1BasePath string = "/ct/v1/"
	// You'd think these would be defined in some library but if so I haven't found it yet
	httpMethodPost = "POST"
	httpMethodGet  = "GET"
)

const (
	// HTTP content type header
	contentTypeHeader string = "Content-Type"
	// MIME content type for JSON
	contentTypeJSON string = "application/json"
	// The name of the JSON response map key in get-roots responses
	jsonMapKeyCertificates string = "certificates"
	// Logging level for debug verbose logs
	logVerboseLevel glog.Level = 2
	// Max number of entries we allow in a get-entries request
	maxGetEntriesAllowed int64 = 50
	// The name of the get-entries start parameter
	getEntriesParamStart = "start"
	// The name of the get-entries end parameter
	getEntriesParamEnd = "end"
)

// CTRequestHandlers provides HTTP handler functions for CT V1 as defined in RFC 6962
// and functionality to translate CT client requests into forms that can be served by a
// log backend RPC service.
type CTRequestHandlers struct {
	// logID is the tree ID that identifies this log in node storage
	logID int64
	// trustedRoots is a pool of certificates that defines the roots the CT log will accept
	trustedRoots *PEMCertPool
	// rpcClient is the client used to communicate with the trillian backend
	rpcClient trillian.TrillianLogClient
	// logKeyManager holds the keys this log needs to sign objects
	logKeyManager crypto.KeyManager
	// rpcDeadline is the deadline that will be set on all backend RPC requests
	rpcDeadline time.Duration
	// timeSource is a util.TimeSource that can be injected for testing
	timeSource util.TimeSource
}

// NewCTRequestHandlers creates a new instance of CTRequestHandlers. They must still
// be registered by calling RegisterCTHandlers()
func NewCTRequestHandlers(logID int64, trustedRoots *PEMCertPool, rpcClient trillian.TrillianLogClient, km crypto.KeyManager, rpcDeadline time.Duration, timeSource util.TimeSource) *CTRequestHandlers {
	return &CTRequestHandlers{logID, trustedRoots, rpcClient, km, rpcDeadline, timeSource}
}

func pathFor(req string) string {
	return ctV1BasePath + req
}

// addChainRequest is a struct for parsing JSON add-chain requests. See RFC 6962 Sections 4.1 and 4.2
type addChainRequest struct {
	Chain []string
}

// addChainResponse is a struct for marshalling add-chain responses. See RFC 6962 Sections 4.1 and 4.2
type addChainResponse struct {
	SctVersion int    `json:"sct_version"`
	ID         string `json:"id"`
	Timestamp  uint64 `json:"timestamp"`
	Extensions string `json:"extensions"`
	Signature  string `json:"signature"`
}

// getEntriesEntry is a struct that represents one element in a get-entries response
type getEntriesEntry struct {
	LeafInput []byte `json:"leaf_input"`
	ExtraData []byte `json:"extra_data"`
}

// getEntriesResponse is a struct for marshalling get-entries respsonses. See RFC6962 Section 4.6
type getEntriesResponse struct {
	Entries []getEntriesEntry `json:"entries"`
}

func parseBodyAsJSONChain(w http.ResponseWriter, r *http.Request) (addChainRequest, error) {
	body, err := ioutil.ReadAll(r.Body)

	if err != nil {
		glog.V(logVerboseLevel).Infof("Failed to read request body: %v", err)
		sendHttpError(w, http.StatusBadRequest, err)
		return addChainRequest{}, err
	}

	var req addChainRequest
	if err := json.Unmarshal(body, &req); err != nil {
		glog.V(logVerboseLevel).Infof("Failed to parse request body: %v", err)
		sendHttpError(w, http.StatusBadRequest, fmt.Errorf("unmarshal failed with %v on %s", err, body))
		return addChainRequest{}, err
	}

	// The cert chain is not allowed to be empty. We'll defer other validation for later
	if len(req.Chain) == 0 {
		glog.V(logVerboseLevel).Infof("Request chain is empty: %s", body)
		sendHttpError(w, http.StatusBadRequest, errors.New("request chain cannot be empty"))
		return addChainRequest{}, errors.New("cert chain was empty")
	}

	return req, nil
}

// enforceMethod checks that the request method is the one we expect and does some additional
// common request validation. If it returns false then the http status has been set appropriately
// and no further action is needed
func enforceMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return false
	}

	// For GET requests all params come as form encoded so we might as well parse them now.
	// POSTs will decode the raw request body as JSON later.
	if r.Method == httpMethodGet {
		if err := r.ParseForm(); err != nil {
			sendHttpError(w, http.StatusBadRequest, err)
			return false
		}
	}

	return true
}

// addChainInternal is called by add-chain and add-pre-chain as the logic involved in
// processing these requests is almost identical
func addChainInternal(w http.ResponseWriter, r *http.Request, c CTRequestHandlers, isPrecert bool) {
	if !enforceMethod(w, r, httpMethodPost) {
		// HTTP status code was already set
		return
	}

	addChainRequest, err := parseBodyAsJSONChain(w, r)

	if err != nil {
		// HTTP status code was already set
		return
	}

	// We already checked that the chain is not empty so can move on to verification
	validPath := verifyAddChain(addChainRequest, w, *c.trustedRoots, isPrecert)

	if validPath == nil {
		// Chain rejected by verify. HTTP status code was already set
		return
	}

	// Build up the SCT and MerkleTreeLeaf. The SCT will be returned to the client and
	// the leaf will become part of the data sent to the backend.
	var merkleTreeLeaf ct.MerkleTreeLeaf
	var sct ct.SignedCertificateTimestamp

	if isPrecert {
		merkleTreeLeaf, sct, err = SignV1SCTForPrecertificate(c.logKeyManager, validPath[0], c.timeSource.Now())
	} else {
		merkleTreeLeaf, sct, err = SignV1SCTForCertificate(c.logKeyManager, validPath[0], c.timeSource.Now())
	}

	if err != nil {
		glog.Warningf("Failed to create / serialize SCT or Merkle leaf: %v %v", sct, err)
		sendHttpError(w, http.StatusInternalServerError, err)
		return
	}

	// Inputs validated, pass the request on to the back end after hashing and serializing
	// the data for the request
	leafProto, err := buildLeafProtoForAddChain(merkleTreeLeaf, validPath)

	if err != nil {
		// Failure reason already logged
		sendHttpError(w, http.StatusInternalServerError, err)
		return
	}

	request := trillian.QueueLeavesRequest{LogId: c.logID, Leaves: []*trillian.LeafProto{&leafProto}}

	ctx, _ := context.WithDeadline(context.Background(), getRPCDeadlineTime(c))

	response, err := c.rpcClient.QueueLeaves(ctx, &request)

	if err != nil || !rpcStatusOK(response.GetStatus()) {
		// TODO(Martin2112): Possibly cases where the request we sent to the backend is invalid
		// which isn't really an internal server error.
		// Request failed on backend
		sendHttpError(w, http.StatusInternalServerError, err)
		return
	}

	// Success. We can now build and marshal the JSON response and write it out
	err = marshalAndWriteAddChainResponse(sct, c.logKeyManager, w)

	if err != nil {
		// reason is logged and http status is already set
		// TODO(Martin2112): Record failure for monitoring when it's implemented
	}
}

// All the handlers are wrapped so they have access to the RPC client and other context
// TODO(Martin2112): Doesn't properly handle duplicate submissions yet but the backend
// needs this to be implemented before we can do it here
func wrappedAddChainHandler(c CTRequestHandlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		addChainInternal(w, r, c, false)
	}
}

func wrappedAddPreChainHandler(c CTRequestHandlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		addChainInternal(w, r, c, true)
	}
}

func wrappedGetSTHHandler(rpcClient trillian.TrillianLogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !enforceMethod(w, r, httpMethodGet) {
			return
		}

		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
	}
}

func wrappedGetSTHConsistencyHandler(rpcClient trillian.TrillianLogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !enforceMethod(w, r, httpMethodGet) {
			return
		}

		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
	}
}

func wrappedGetProofByHashHandler(rpcClient trillian.TrillianLogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !enforceMethod(w, r, httpMethodGet) {
			return
		}

		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
	}
}

func wrappedGetEntriesHandler(c CTRequestHandlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !enforceMethod(w, r, httpMethodGet) {
			return
		}

		// The first job is to parse the params and make sure they're sensible. We just make
		// sure the range is valid. We don't do an extra roundtrip to get the current tree
		// size and prefer to let the backend handle this case
		startIndex, endIndex, err := parseAndValidateGetEntriesRange(r, maxGetEntriesAllowed)

		if err != nil {
			glog.Warningf("Bad range on get-entries request: %v", err)
			sendHttpError(w, http.StatusBadRequest, err)
			return
		}

		// Now make a request to the backend to get the relevant leaves
		requestIndices := buildIndicesForRange(startIndex, endIndex)
		request := trillian.GetLeavesByIndexRequest{LogId: c.logID, LeafIndex: requestIndices}

		ctx, _ := context.WithDeadline(context.Background(), getRPCDeadlineTime(c))

		response, err := c.rpcClient.GetLeavesByIndex(ctx, &request)

		if err != nil || !rpcStatusOK(response.GetStatus()) {
			sendHttpError(w, http.StatusInternalServerError, fmt.Errorf("RPC failed, possible extra info: %v", err))
			return
		}

		// Apply additional checks on the response to make sure we got a contiguous leaf range.
		// It's allowed by the RFC for the backend to truncate the range in cases where the
		// range exceeds the tree size etc. so we could get fewer leaves than we requested but
		// never more and never anything outside the requested range.
		if expected, got := len(requestIndices), len(response.Leaves); got > expected {
			glog.Warningf("Backend returned more leaves (%d) than requested: (%d)", got, expected)
			sendHttpError(w, http.StatusInternalServerError, fmt.Errorf("Backend returned too many leaves: %d", got))
			return
		}

		if err := isResponseContiguousRange(response, startIndex, endIndex); err != nil {
			glog.Warningf("Backend get-entries range received from backend non contiguous: %v", err)
			sendHttpError(w, http.StatusInternalServerError, err)
			return
		}

		// Now we've checked the response and it seems to be valid we need to serialize the
		// leaves in JSON format. Doing a round trip via the leaf deserializer gives us another
		// chance to prevent bad / corrupt data from reaching the client.
		jsonResponse, err := marshalGetEntriesResponse(response)

		if err != nil {
			glog.Warningf("Failed to process leaves returned from backend: %v", err)
			sendHttpError(w, http.StatusInternalServerError, err)
			return
		}

		w.Header().Set(contentTypeHeader, contentTypeJSON)
		jsonData, err := json.Marshal(&jsonResponse)

		if err != nil {
			glog.Warningf("Failed to marshal get-entries resp: %v", jsonResponse)
			sendHttpError(w, http.StatusInternalServerError, err)
			return
		}

		_, err = w.Write(jsonData)

		if err != nil {
			glog.Warningf("Failed to write get-entries resp: %v", jsonResponse)
			// Probably too late for this as headers might have been written but we don't know for sure
			sendHttpError(w, http.StatusInternalServerError, err)
			return
		}
	}
}

func wrappedGetRootsHandler(trustedRoots *PEMCertPool, rpcClient trillian.TrillianLogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !enforceMethod(w, r, httpMethodGet) {
			return
		}

		jsonMap := make(map[string]interface{})

		rawCerts := make([][]byte, 0, len(trustedRoots.RawCertificates()))

		// Pull out the raw certificates from the parsed versions
		for _, cert := range trustedRoots.RawCertificates() {
			rawCerts = append(rawCerts, cert.Raw)
		}

		jsonMap[jsonMapKeyCertificates] = rawCerts
		enc := json.NewEncoder(w)
		err := enc.Encode(jsonMap)

		if err != nil {
			glog.Warningf("get_roots failed: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
}

func wrappedGetEntryAndProofHandler(rpcClient trillian.TrillianLogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !enforceMethod(w, r, httpMethodGet) {
			return
		}

		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
	}
}

// RegisterCTHandlers registers a HandleFunc for all of the RFC6962 defined methods.
// TODO(Martin2112): This registers on default ServeMux, might need more flexibility?
func (c CTRequestHandlers) RegisterCTHandlers() {
	http.HandleFunc(pathFor("add-chain"), wrappedAddChainHandler(c))
	http.HandleFunc(pathFor("add-pre-chain"), wrappedAddPreChainHandler(c))
	http.HandleFunc(pathFor("get-sth"), wrappedGetSTHHandler(c.rpcClient))
	http.HandleFunc(pathFor("get-sth-consistency"), wrappedGetSTHConsistencyHandler(c.rpcClient))
	http.HandleFunc(pathFor("get-proof-by-hash"), wrappedGetProofByHashHandler(c.rpcClient))
	http.HandleFunc(pathFor("get-entries"), wrappedGetEntriesHandler(c))
	http.HandleFunc(pathFor("get-roots"), wrappedGetRootsHandler(c.trustedRoots, c.rpcClient))
	http.HandleFunc(pathFor("get-entry-and-proof"), wrappedGetEntryAndProofHandler(c.rpcClient))
}

// Generates a custom error page to give more information on why something didn't work
// TODO(Martin2112): Not sure if we want to expose any detail or not
func sendHttpError(w http.ResponseWriter, statusCode int, err error) {
	http.Error(w, fmt.Sprintf("%s\n%v", http.StatusText(statusCode), err), statusCode)
}

// getRPCDeadlineTime calculates the future time an RPC should expire based on our config
func getRPCDeadlineTime(c CTRequestHandlers) time.Time {
	return c.timeSource.Now().Add(c.rpcDeadline)
}

func rpcStatusOK(status *trillian.TrillianApiStatus) bool {
	return status != nil && status.StatusCode == trillian.TrillianApiStatusCode_OK
}

// verifyAddChain is used by add-chain and add-pre-chain. It does the checks that the supplied
// cert is of the correct type and chains to a trusted root.
// TODO(Martin2112): This may not implement all the RFC requirements. Check what is provided
// by fixchain (called by this code) plus the ones here to make sure that it is compliant.
func verifyAddChain(req addChainRequest, w http.ResponseWriter, trustedRoots PEMCertPool, expectingPrecert bool) []*x509.Certificate {
	// We already checked that the chain is not empty so can move on to verification
	validPath, err := ValidateChain(req.Chain, trustedRoots)

	if err != nil {
		// We rejected it because the cert failed checks or we could not find a path to a root etc.
		// Lots of possible causes for errors
		glog.Warningf("Chain failed to verify: %v", req)
		sendHttpError(w, http.StatusBadRequest, err)
		return nil
	}

	isPrecert, err := IsPrecertificate(validPath[0])

	if err != nil {
		glog.Warningf("Precert test failed: %v", err)
		sendHttpError(w, http.StatusBadRequest, err)
		return nil
	}

	// The type of the leaf must match the one the handler expects
	if isPrecert != expectingPrecert {
		if expectingPrecert {
			glog.Warningf("Cert (or precert with invalid CT ext) submitted as precert chain: %v", req)
		} else {
			glog.Warningf("Precert (or cert with invalid CT ext) submitted as cert chain: %v", req)
		}
		sendHttpError(w, http.StatusBadRequest, err)
		return nil
	}

	return validPath
}

// marshalLogIDAndSignatureForResponse is used by add-chain and add-pre-chain. It formats the
// signature and log id ready to send to the client.
func marshalLogIDAndSignatureForResponse(sct ct.SignedCertificateTimestamp, km crypto.KeyManager) ([sha256.Size]byte, string, error) {
	logID, err := GetCTLogID(km)

	if err != nil {
		return [32]byte{}, "", fmt.Errorf("failed to marshal logID: %v", err)
	}

	signature, err := sct.Signature.Base64String()

	if err != nil {
		return [32]byte{}, "", fmt.Errorf("failed to marshal signature: %v %v", sct.Signature, err)
	}

	return logID, signature, nil
}

// buildLeafProtoForAddChain is also used by add-pre-chain and does the hashing to build a
// LeafProto that will be sent to the backend
func buildLeafProtoForAddChain(merkleLeaf ct.MerkleTreeLeaf, certChain []*x509.Certificate) (trillian.LeafProto, error) {
	var leafBuffer bytes.Buffer
	if err := WriteMerkleTreeLeaf(&leafBuffer, merkleLeaf); err != nil {
		glog.Warningf("Failed to serialize merkle leaf: %v", err)
		return trillian.LeafProto{}, err
	}

	var logEntryBuffer bytes.Buffer
	logEntry := NewCTLogEntry(merkleLeaf, certChain)
	if err := logEntry.Serialize(&logEntryBuffer); err != nil {
		glog.Warningf("Failed to serialize log entry: %v", err)
		return trillian.LeafProto{}, err
	}

	// leafHash is a crosscheck on the data we're sending in the leaf buffer. The backend
	// does the tree hashing.
	leafHash := sha256.Sum256(leafBuffer.Bytes())

	return trillian.LeafProto{LeafHash: leafHash[:], LeafData: leafBuffer.Bytes(), ExtraData: logEntryBuffer.Bytes()}, nil
}

// marshalAndWriteAddChainResponse is used by add-chain and add-pre-chain to create and write
// the JSON response to the client
func marshalAndWriteAddChainResponse(sct ct.SignedCertificateTimestamp, km crypto.KeyManager, w http.ResponseWriter) error {
	logID, signature, err := marshalLogIDAndSignatureForResponse(sct, km)

	if err != nil {
		glog.Warningf("failed to marshal for response: %v", err)
		return err
	}

	resp := addChainResponse{
		SctVersion: int(sct.SCTVersion),
		Timestamp:  sct.Timestamp,
		ID:         base64.StdEncoding.EncodeToString(logID[:]),
		Extensions: "",
		Signature:  signature}

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	jsonData, err := json.Marshal(&resp)

	if err != nil {
		glog.Warningf("Failed to marshal add-chain resp: %v", resp)
		sendHttpError(w, http.StatusInternalServerError, err)
		return err
	}

	_, err = w.Write(jsonData)

	if err != nil {
		glog.Warningf("Failed to write add-chain resp: %v", resp)
		// Probably too late for this as headers might have been written but we don't know for sure
		sendHttpError(w, http.StatusInternalServerError, err)
		return err
	}

	return nil
}

func parseAndValidateGetEntriesRange(r *http.Request, maxAllowedRange int64) (int64, int64, error) {
	startIndex, err := strconv.ParseInt(r.FormValue(getEntriesParamStart), 10, 64)

	if err != nil {
		return 0, 0, err
	}

	endIndex, err := strconv.ParseInt(r.FormValue(getEntriesParamEnd), 10, 64)

	if err != nil {
		return 0, 0, err
	}

	return validateStartAndEnd(startIndex, endIndex, maxAllowedRange)
}

// validateStartAndEnd applies validation to the range params for get-entries. Either returns
// the parameters to be used (which could be a subset of the request input though it
// currently never is) or an error that describes why the parameters are not acceptable.
func validateStartAndEnd(start, end, maxRange int64) (int64, int64, error) {
	if start < 0 || end < 0 {
		return 0, 0, fmt.Errorf("start (%d) and end (%d) parameters must be >= 0", start, end)
	}

	if start > end {
		return 0, 0, fmt.Errorf("start (%d) and end (%d) is not a valid range", start, end)
	}

	numEntries := end - start + 1

	if numEntries > maxRange {
		return 0, 0, fmt.Errorf("requesting %d entries but we only allow up to %d", numEntries, maxRange)
	}

	return start, end, nil
}

// buildIndicesForRange expands the range out, the backend allows for non contiguous leaf fetches
// but the CT spec doesn't. The input values should have been checked for consistency before calling
// this.
func buildIndicesForRange(start, end int64) []int64 {
	indices := make([]int64, 0, end-start+1)
	for i := start; i <= end; i++ {
		indices = append(indices, i)
	}

	return indices
}

// isResponseContiguousRange checks that the response has a contiguous range of leaves and
// that it is a subset or equal to the requested range. This is additional protection against
// backend bugs. Returns nil if the response looks valid.
func isResponseContiguousRange(response *trillian.GetLeavesByIndexResponse, start, end int64) error {
	for li, l := range response.Leaves {
		if li > 0 && response.Leaves[li].LeafIndex-response.Leaves[li-1].LeafIndex != 1 {
			return fmt.Errorf("backend returned non contiguous leaves: %v %v", response.Leaves[li-1], response.Leaves[li])
		}

		if l.LeafIndex < start || l.LeafIndex > end {
			return fmt.Errorf("backend returned leaf:%d outside requested range:%d, %d", l.LeafIndex, start, end)
		}
	}

	return nil
}

// marshalGetEntriesResponse does the conversion from the backend response to the one we need for
// an RFC compliant JSON response to the client.
func marshalGetEntriesResponse(rpcResponse *trillian.GetLeavesByIndexResponse) (getEntriesResponse, error) {
	jsonResponse := getEntriesResponse{}

	for _, leaf := range rpcResponse.Leaves {
		// We're only deserializing it to ensure it's valid, don't need the result. We still
		// return the data if it fails to deserialize as otherwise the root hash could not
		// be verified. However this indicates a potentially serious failure in log operation
		// or data storage that should be investigated.
		if _, err := ct.ReadMerkleTreeLeaf(bytes.NewBuffer(leaf.LeafData)); err != nil {
			// TODO(Martin2112): Hook this up to monitoring when implemented
			glog.Warningf("Failed to deserialize merkle leaf from backend: %d", leaf.LeafIndex)
		}

		jsonResponse.Entries = append(jsonResponse.Entries, getEntriesEntry{
			LeafInput: leaf.LeafData,
			ExtraData: leaf.ExtraData})
	}

	return jsonResponse, nil
}
