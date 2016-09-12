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
	// The name of the get-proof-by-hash parameter
	getProofParamHash = "hash"
	// The name of the get-proof-by-hash tree size parameter
	getProofParamTreeSize = "tree_size"
	// The name of the get-sth-consistency first snapshot param
	getSTHConsistencyParamFirst = "first"
	// The name of the get-sth-consistency second snapshot param
	getSTHConsistencyParamSecond = "second"
	// The name of the get-entry-and-proof index parameter
	getEntryAndProofParamLeafIndex = "leaf_index"
	// The name of the get-entry-and-proof tree size paramter
	getEntryAndProofParamTreeSize = "tree_size"
)

// appHandler is a type for simplifying and centralizing error handling from http handlers
type appHandler func(http.ResponseWriter, *http.Request) (int, error)

// ServeHTTP is an adapter from appHandler to the http framework
func (fn appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	status, err := fn(w, r)

	if err != nil {
		glog.Warningf("handler error: %v", err)
		sendHttpError(w, status, err)
	}

	// Additional check, for consistency the handler must return an error for non 200 status
	if status != http.StatusOK {
		glog.Warningf("handler non 200 without error: %d %v", status, err)
		sendHttpError(w, http.StatusInternalServerError, fmt.Errorf("http handler misbehaved, status: %d", status))
	}
}

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

// getSTHResponse is a struct for marshalling get-sth responses. See RFC 6962 Section 4.3
type getSTHResponse struct {
	TreeSize        int64  `json:"tree_size"`
	TimestampMillis int64  `json:"timestamp"`
	RootHash        []byte `json:"sha256_root_hash"`
	Signature       []byte `json:"tree_head_signature"`
}

// getProofByHashResponse is a struct for marshalling get-proof-by-hash responses. See RFC 6962
// section 4.5
type getProofByHashResponse struct {
	LeafIndex int64    `json:"leaf_index"`
	AuditPath [][]byte `json:"audit_path"`
}

// getSTHConsistencyResponse is a struct for mashalling get-sth-consistency responses. See
// RFC 6962 section 4.4
type getSTHConsistencyResponse struct {
	Consistency [][]byte `json:"consistency"`
}

// getEntryAndProofResponse is a struct for marshalling get-entry-and-proof responses. See
// RFC 6962 Section 4.8
type getEntryAndProofResponse struct {
	LeafInput []byte `json:"leaf_input"`
	ExtraData []byte `json:"extra_data"`
	AuditPath [][]byte `json:"audit_path"`
}

func parseBodyAsJSONChain(w http.ResponseWriter, r *http.Request) (addChainRequest, error) {
	body, err := ioutil.ReadAll(r.Body)

	if err != nil {
		glog.V(logVerboseLevel).Infof("Failed to read request body: %v", err)
		return addChainRequest{}, err
	}

	var req addChainRequest
	if err := json.Unmarshal(body, &req); err != nil {
		glog.V(logVerboseLevel).Infof("Failed to parse request body: %v", err)
		return addChainRequest{}, err
	}

	// The cert chain is not allowed to be empty. We'll defer other validation for later
	if len(req.Chain) == 0 {
		glog.V(logVerboseLevel).Infof("Request chain is empty: %s", body)
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
func addChainInternal(w http.ResponseWriter, r *http.Request, c CTRequestHandlers, isPrecert bool) (int, error) {
	if !enforceMethod(w, r, httpMethodPost) {
		// HTTP status code was already set
		return http.StatusMethodNotAllowed, fmt.Errorf("method not allowed: %s", r.Method)
	}

	addChainRequest, err := parseBodyAsJSONChain(w, r)

	if err != nil {
		return http.StatusBadRequest, err
	}

	// We already checked that the chain is not empty so can move on to verification
	validPath, err := verifyAddChain(addChainRequest, w, *c.trustedRoots, isPrecert)

	if err != nil {
		// Chain rejected by verify.
		return http.StatusBadRequest, err
	}

	// Build up the SCT and MerkleTreeLeaf. The SCT will be returned to the client and
	// the leaf will become part of the data sent to the backend.
	var merkleTreeLeaf ct.MerkleTreeLeaf
	var sct ct.SignedCertificateTimestamp

	if isPrecert {
		merkleTreeLeaf, sct, err = signV1SCTForPrecertificate(c.logKeyManager, validPath[0], c.timeSource.Now())
	} else {
		merkleTreeLeaf, sct, err = signV1SCTForCertificate(c.logKeyManager, validPath[0], c.timeSource.Now())
	}

	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to create / serialize SCT or Merkle leaf: %v %v", sct, err)
	}

	// Inputs validated, pass the request on to the back end after hashing and serializing
	// the data for the request
	leafProto, err := buildLeafProtoForAddChain(merkleTreeLeaf, validPath)

	if err != nil {
		// Failure reason already logged
		return http.StatusInternalServerError, err
	}

	request := trillian.QueueLeavesRequest{LogId: c.logID, Leaves: []*trillian.LeafProto{&leafProto}}

	ctx, _ := context.WithDeadline(context.Background(), getRPCDeadlineTime(c))

	response, err := c.rpcClient.QueueLeaves(ctx, &request)

	if err != nil || !rpcStatusOK(response.GetStatus()) {
		// TODO(Martin2112): Possibly cases where the request we sent to the backend is invalid
		// which isn't really an internal server error.
		// Request failed on backend
		return http.StatusInternalServerError, err
	}

	// Success. We can now build and marshal the JSON response and write it out
	err = marshalAndWriteAddChainResponse(sct, c.logKeyManager, w)

	if err != nil {
		// reason is logged and http status is already set
		// TODO(Martin2112): Record failure for monitoring when it's implemented
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

// All the handlers are wrapped so they have access to the RPC client and other context
// TODO(Martin2112): Doesn't properly handle duplicate submissions yet but the backend
// needs this to be implemented before we can do it here
func wrappedAddChainHandler(c CTRequestHandlers) appHandler {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		return addChainInternal(w, r, c, false)
	}
}

func wrappedAddPreChainHandler(c CTRequestHandlers) appHandler {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		return addChainInternal(w, r, c, true)
	}
}

func wrappedGetSTHHandler(c CTRequestHandlers) appHandler {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		if !enforceMethod(w, r, httpMethodGet) {
			return http.StatusMethodNotAllowed, fmt.Errorf("method not allowed: %s", r.Method)
		}

		request := trillian.GetLatestSignedLogRootRequest{LogId: c.logID}
		ctx, _ := context.WithDeadline(context.Background(), getRPCDeadlineTime(c))
		response, err := c.rpcClient.GetLatestSignedLogRoot(ctx, &request)

		if err != nil || !rpcStatusOK(response.GetStatus()) {
			return http.StatusInternalServerError, errors.New("backend rpc failed")
		}

		if treeSize := response.GetSignedLogRoot().TreeSize; treeSize < 0 {
			return http.StatusInternalServerError, fmt.Errorf("bad tree size from backend: %d", treeSize)
		}

		if hashSize := len(response.GetSignedLogRoot().RootHash); hashSize != sha256.Size {
			return http.StatusInternalServerError, fmt.Errorf("bad hash size from backend expecting: %d got %d", sha256.Size, hashSize)
		}

		// Jump through Go hoops because we're mixing arrays and slices, we checked the size above
		// so it should exactly fit what we copy into it
		var hashArray [sha256.Size]byte
		copy(hashArray[:], response.GetSignedLogRoot().RootHash)

		// Build the CT STH object ready for signing
		sth := ct.SignedTreeHead{TreeSize: uint64(response.GetSignedLogRoot().TreeSize),
			Timestamp:      uint64(response.GetSignedLogRoot().TimestampNanos / 1000 / 1000),
			SHA256RootHash: hashArray}

		// Serialize and sign the STH and make sure this succeeds
		err = signV1TreeHead(c.logKeyManager, &sth)

		if err != nil || len(sth.TreeHeadSignature.Signature) == 0 {
			return http.StatusInternalServerError, fmt.Errorf("invalid tree size in get sth: %v", err)
		}

		// Now build the final result object that will be marshalled to JSON
		jsonResponse := convertSTHForClientResponse(sth)

		w.Header().Set(contentTypeHeader, contentTypeJSON)
		jsonData, err := json.Marshal(&jsonResponse)

		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("failed to marshall response: %v %v", jsonResponse, err)
		}

		_, err = w.Write(jsonData)

		if err != nil {
			// Probably too late for this as headers might have been written but we don't know for sure
			return http.StatusInternalServerError, err
		}

		return http.StatusOK, nil
	}
}

func wrappedGetSTHConsistencyHandler(c CTRequestHandlers) appHandler {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		if !enforceMethod(w, r, httpMethodGet) {
			return http.StatusMethodNotAllowed, fmt.Errorf("method not allowed: %s", r.Method)
		}

		first, second, err := parseAndValidateGetSTHConsistencyRange(r)

		if err != nil {
			return http.StatusBadRequest, err
		}

		request := trillian.GetConsistencyProofRequest{LogId:c.logID, FirstTreeSize:first, SecondTreeSize:second}
		ctx, _ := context.WithDeadline(context.Background(), getRPCDeadlineTime(c))
		response, err := c.rpcClient.GetConsistencyProof(ctx, &request)

		if err != nil || !rpcStatusOK(response.GetStatus()) {
			return http.StatusInternalServerError, err
		}

		// Additional sanity checks, none of the hashes in the returned path should be empty
		if !checkAuditPath(response.Proof.ProofNode) {
			return http.StatusInternalServerError, fmt.Errorf("backend returned invalid proof: %v", response.Proof)
		}

		// We got a valid response from the server. Marshall it as JSON and return it to the client
		jsonResponse := getSTHConsistencyResponse{Consistency:auditPathFromProto(response.Proof.ProofNode)}

		w.Header().Set(contentTypeHeader, contentTypeJSON)
		jsonData, err := json.Marshal(&jsonResponse)

		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-sth-consistency resp: %v because %v", jsonResponse, err)
		}

		_, err = w.Write(jsonData)

		if err != nil {
			// Probably too late for this as headers might have been written but we don't know for sure
			return http.StatusInternalServerError, fmt.Errorf("failed to write get-sth-consistency resp: %v because %v", jsonResponse, err)
		}

		return http.StatusOK, nil
	}
}

func wrappedGetProofByHashHandler(c CTRequestHandlers) appHandler {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		if !enforceMethod(w, r, httpMethodGet) {
			return http.StatusMethodNotAllowed, fmt.Errorf("method not allowed: %s", r.Method)
		}

		hash := r.FormValue(getProofParamHash)

		// Accept any non empty hash that decodes from base64 and let the backend validate it further
		if len(hash) == 0 {
			return http.StatusBadRequest, errors.New("get-proof-by-hash: missing / empty hash param for get-proof-by-hash")
		}

		leafHash, err := base64.StdEncoding.DecodeString(hash)

		if err != nil {
			return http.StatusBadRequest, fmt.Errorf("get-proof-by-hash: invalid base64 hash: %v", err)
		}

		treeSize, err := strconv.ParseInt(r.FormValue(getProofParamTreeSize), 10, 64)

		if err != nil || treeSize < 1 {
			return http.StatusBadRequest, fmt.Errorf("get-proof-by-hash: missing or invalid tree_size: %v", r.FormValue(getProofParamTreeSize))
		}

		// Per RFC 6962 section 4.5 the API returns a single proof. This should be the lowest leaf index
		// Because we request order by sequence and we only passed one hash then the first result is
		// the correct proof to return
		rpcRequest := trillian.GetInclusionProofByHashRequest{LogId: c.logID,
			LeafHash: leafHash,
			TreeSize:treeSize,
			OrderBySequence:true}
		ctx, _ := context.WithDeadline(context.Background(), getRPCDeadlineTime(c))
		response, err := c.rpcClient.GetInclusionProofByHash(ctx, &rpcRequest)

		if err != nil || !rpcStatusOK(response.GetStatus()) {
			return http.StatusInternalServerError, fmt.Errorf("get-proof-by-hash: RPC failed, possible extra info: %v", err)
		}

		// Additional sanity checks, none of the hashes in the returned path should be empty
		if !checkAuditPath(response.Proof[0].ProofNode) {
			return http.StatusInternalServerError, fmt.Errorf("get-proof-by-hash: backend returned invalid proof: %v", response.Proof[0])
		}

		// All checks complete, marshall and return the response
		proofResponse := getProofByHashResponse{LeafIndex: response.Proof[0].LeafIndex, AuditPath: auditPathFromProto(response.Proof[0].ProofNode)}

		w.Header().Set(contentTypeHeader, contentTypeJSON)
		jsonData, err := json.Marshal(&proofResponse)

		if err != nil {
			glog.Warningf("Failed to marshal get-proof-by-hash resp: %v", proofResponse)
			return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-proof-by-hash resp: %v, error: %v", proofResponse, err)
		}

		_, err = w.Write(jsonData)

		if err != nil {
			// Probably too late for this as headers might have been written but we don't know for sure
			return http.StatusInternalServerError, fmt.Errorf("failed to write get-proof-by-hash resp: %v", proofResponse)
		}

		return http.StatusOK, nil
	}
}

func wrappedGetEntriesHandler(c CTRequestHandlers) appHandler {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		if !enforceMethod(w, r, httpMethodGet) {
			return http.StatusMethodNotAllowed, fmt.Errorf("method not allowed: %s", r.Method)
		}

		// The first job is to parse the params and make sure they're sensible. We just make
		// sure the range is valid. We don't do an extra roundtrip to get the current tree
		// size and prefer to let the backend handle this case
		startIndex, endIndex, err := parseAndValidateGetEntriesRange(r, maxGetEntriesAllowed)

		if err != nil {
			return http.StatusBadRequest, fmt.Errorf("bad range on get-entries request: %v", err)
		}

		// Now make a request to the backend to get the relevant leaves
		requestIndices := buildIndicesForRange(startIndex, endIndex)
		request := trillian.GetLeavesByIndexRequest{LogId: c.logID, LeafIndex: requestIndices}

		ctx, _ := context.WithDeadline(context.Background(), getRPCDeadlineTime(c))

		response, err := c.rpcClient.GetLeavesByIndex(ctx, &request)

		if err != nil || !rpcStatusOK(response.GetStatus()) {
			return http.StatusInternalServerError, fmt.Errorf("get-entries: RPC failed, possible extra info: %v", err)
		}

		// Apply additional checks on the response to make sure we got a contiguous leaf range.
		// It's allowed by the RFC for the backend to truncate the range in cases where the
		// range exceeds the tree size etc. so we could get fewer leaves than we requested but
		// never more and never anything outside the requested range.
		if expected, got := len(requestIndices), len(response.Leaves); got > expected {
			return http.StatusInternalServerError, fmt.Errorf("backend returned too many leaves: %d v %d", got, expected)
		}

		if err := isResponseContiguousRange(response, startIndex, endIndex); err != nil {
			return http.StatusInternalServerError, fmt.Errorf("backend get-entries range received from backend non contiguous: %v", err)
		}

		// Now we've checked the response and it seems to be valid we need to serialize the
		// leaves in JSON format. Doing a round trip via the leaf deserializer gives us another
		// chance to prevent bad / corrupt data from reaching the client.
		jsonResponse, err := marshalGetEntriesResponse(response)

		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("failed to process leaves returned from backend: %v", err)
		}

		w.Header().Set(contentTypeHeader, contentTypeJSON)
		jsonData, err := json.Marshal(&jsonResponse)

		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-entries resp: %v because: %v", jsonResponse, err)
		}

		_, err = w.Write(jsonData)

		if err != nil {

			// Probably too late for this as headers might have been written but we don't know for sure
			return http.StatusInternalServerError, fmt.Errorf("failed to write get-entries resp: %v because: %v", jsonResponse, err)
		}

		return http.StatusOK, nil
	}
}

func wrappedGetRootsHandler(trustedRoots *PEMCertPool) appHandler {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		if !enforceMethod(w, r, httpMethodGet) {
			return http.StatusMethodNotAllowed, fmt.Errorf("method not allowed: %s", r.Method)
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
			return http.StatusInternalServerError, fmt.Errorf("get-roots failed with: %v", err)
		}

		return http.StatusOK, nil
	}
}

// See RFC 6962 Section 4.8. This is mostly used for debug purposes rather than by normal
// CT clients.
func wrappedGetEntryAndProofHandler(c CTRequestHandlers) appHandler {
	return func(w http.ResponseWriter, r *http.Request) (int, error) {
		if !enforceMethod(w, r, httpMethodGet) {
			return http.StatusMethodNotAllowed, fmt.Errorf("method not allowed: %s", r.Method)
		}

		// Ensure both numeric params are present and look reasonable.
		leafIndex, treeSize, err := parseAndValidateGetEntryAndProofParams(r)

		if err != nil {
			return http.StatusBadRequest, err
		}

		getEntryAndProofRequest := trillian.GetEntryAndProofRequest{LogId:c.logID, LeafIndex:leafIndex, TreeSize:treeSize}
		ctx, _ := context.WithDeadline(context.Background(), getRPCDeadlineTime(c))
		response, err := c.rpcClient.GetEntryAndProof(ctx, &getEntryAndProofRequest)

		if err != nil || !rpcStatusOK(response.GetStatus()) {
			return http.StatusInternalServerError, fmt.Errorf("get-entry-and-proof: RPC failed, possible extra info: %v", err)
		}

		// Apply some checks that we got reasonable data from the backend
		if response.Proof == nil || response.Leaf == nil || len(response.Proof.ProofNode) == 0 || len(response.Leaf.LeafData) == 0 {
			return http.StatusInternalServerError, fmt.Errorf("got RPC bad response, possible extra info: %v", response)
		}

		// Build and marshall the response to the client
		jsonResponse := getEntryAndProofResponse{
			LeafInput:response.Leaf.LeafData,
			ExtraData:response.Leaf.ExtraData,
			AuditPath:auditPathFromProto(response.Proof.ProofNode)}

		w.Header().Set(contentTypeHeader, contentTypeJSON)
		jsonData, err := json.Marshal(&jsonResponse)

		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-entry-and-proof resp: %v because: %v", jsonResponse, err)
		}

		_, err = w.Write(jsonData)

		if err != nil {

			// Probably too late for this as headers might have been written but we don't know for sure
			return http.StatusInternalServerError, fmt.Errorf("failed to write get-entry-and-proof resp: %v because: %v", jsonResponse, err)
		}

		return http.StatusOK, nil
	}
}

// RegisterCTHandlers registers a HandleFunc for all of the RFC6962 defined methods.
// TODO(Martin2112): This registers on default ServeMux, might need more flexibility?
func (c CTRequestHandlers) RegisterCTHandlers() {
	http.Handle(pathFor("add-chain"), wrappedAddChainHandler(c))
	http.Handle(pathFor("add-pre-chain"), wrappedAddPreChainHandler(c))
	http.Handle(pathFor("get-sth"), wrappedGetSTHHandler(c))
	http.Handle(pathFor("get-sth-consistency"), wrappedGetSTHConsistencyHandler(c))
	http.Handle(pathFor("get-proof-by-hash"), wrappedGetProofByHashHandler(c))
	http.Handle(pathFor("get-entries"), wrappedGetEntriesHandler(c))
	http.Handle(pathFor("get-roots"), wrappedGetRootsHandler(c.trustedRoots))
	http.Handle(pathFor("get-entry-and-proof"), wrappedGetEntryAndProofHandler(c))
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
func verifyAddChain(req addChainRequest, w http.ResponseWriter, trustedRoots PEMCertPool, expectingPrecert bool) ([]*x509.Certificate, error) {
	// We already checked that the chain is not empty so can move on to verification
	validPath, err := ValidateChain(req.Chain, trustedRoots)

	if err != nil {
		// We rejected it because the cert failed checks or we could not find a path to a root etc.
		// Lots of possible causes for errors
		return nil, fmt.Errorf("chain failed to verify: %v because: %v", req, err)
	}

	isPrecert, err := IsPrecertificate(validPath[0])

	if err != nil {
		return nil, fmt.Errorf("precert test failed: %v", err)
	}

	// The type of the leaf must match the one the handler expects
	if isPrecert != expectingPrecert {
		if expectingPrecert {
			glog.Warningf("Cert (or precert with invalid CT ext) submitted as precert chain: %v", req)
		} else {
			glog.Warningf("Precert (or cert with invalid CT ext) submitted as cert chain: %v", req)
		}
		return nil, fmt.Errorf("cert / precert mismatch: %v", expectingPrecert)
	}

	return validPath, nil
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
	if err := writeMerkleTreeLeaf(&leafBuffer, merkleLeaf); err != nil {
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
		return fmt.Errorf("failed to marshal for response: %v", err)
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
		return fmt.Errorf("failed to marshal add-chain resp: %v because: %v", resp, err)
	}

	_, err = w.Write(jsonData)

	if err != nil {
		return fmt.Errorf("failed to write add-chain resp: %v", resp)
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

func parseAndValidateGetEntryAndProofParams(r *http.Request) (int64, int64, error) {
	leafIndex, err := strconv.ParseInt(r.FormValue(getEntryAndProofParamLeafIndex), 10, 64)

	if err != nil {
		return 0, 0, err
	}

	treeSize, err := strconv.ParseInt(r.FormValue(getEntryAndProofParamTreeSize), 10, 64)

	if err != nil {
		return 0, 0, err
	}

	if treeSize <= 0 {
		return 0, 0, fmt.Errorf("tree_size must be > 0, got: %d", treeSize)
	}

	if leafIndex < 0 {
		return 0, 0, fmt.Errorf("leaf_index must be >= 0, got: %d", treeSize)
	}

	if leafIndex >= treeSize {
		return 0, 0, fmt.Errorf("leaf_index %d out of range for tree of size %d", leafIndex, treeSize)
	}

	return leafIndex, treeSize, nil
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

func parseAndValidateGetSTHConsistencyRange(r *http.Request) (int64, int64, error) {
	first, err := strconv.ParseInt(r.FormValue(getSTHConsistencyParamFirst), 10, 64)

	if err != nil {
		return 0, 0, err
	}

	second, err := strconv.ParseInt(r.FormValue(getSTHConsistencyParamSecond), 10, 64)

	if err != nil {
		return 0, 0, err
	}

	if first <= 0 || second <= 0 {
		return 0, 0, fmt.Errorf("first and second params cannot be <=0: %d %d", first, second)
	}

	if second <= first {
		return 0, 0, fmt.Errorf("invalid first, second params: %d %d", first, second)
	}

	return first, second, nil
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

// convertSTHForClientResponse does some simple marshalling from a properly signed CT object
// to the object we'll use to create the JSON response to a client with the correct RFC
// field names.
func convertSTHForClientResponse(sth ct.SignedTreeHead) getSTHResponse {
	return getSTHResponse{
		TreeSize:        int64(sth.TreeSize),
		RootHash:        sth.SHA256RootHash[:],
		TimestampMillis: int64(sth.Timestamp),
		Signature:       sth.TreeHeadSignature.Signature}
}

// checkAuditPath does a quick scan of the proof we got from the backend for consistency.
// All the hashes should be non zero length.
// TODO(Martin2112): should maybe check they are all the same length and all the expected
// length of the hashes used in the RFC.
func checkAuditPath(path []*trillian.NodeProto) bool {
	for _, pathEntry := range path {
		if len(pathEntry.NodeHash) == 0 {
			return false
		}
	}

	return true
}

// auditPathFromProto converts the path from proof proto to a format we can return in the JSON
// response
func auditPathFromProto(path []*trillian.NodeProto) [][]byte {
	resultPath := make([][]byte, 0, len(path))

	for _, pathEntry := range path {
		resultPath = append(resultPath, pathEntry.NodeHash)
	}

	return resultPath
}
