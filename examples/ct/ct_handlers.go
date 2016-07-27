package ct

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"encoding/base64"
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
	contentTypeHeader      string     = "Content-Type"
	contentTypeJSON        string     = "application/json"
	jsonMapKeyCertificates string     = "certificates"
	logVerboseLevel        glog.Level = 2
)

// CTRequestHandlers provides HTTP handler functions for CT V1 as defined in RFC 6962
// and functionality to translate CT client requests into forms that can be served by a
// log backend RPC service.
type CTRequestHandlers struct {
	logID         int64
	trustedRoots  *PEMCertPool
	rpcClient     trillian.TrillianLogClient
	logKeyManager crypto.KeyManager
	rpcDeadline   time.Duration
	timeSource    util.TimeSource
}

// NewCTRequestHandlers creates a new instance of CTRequestHandlers. They must still
// be registered by calling RegisterCTHandlers()
func NewCTRequestHandlers(logID int64, trustedRoots *PEMCertPool, rpcClient trillian.TrillianLogClient, km crypto.KeyManager, rpcDeadline time.Duration, timeSource util.TimeSource) *CTRequestHandlers {
	return &CTRequestHandlers{logID, trustedRoots, rpcClient, km, rpcDeadline, timeSource}
}

func pathFor(req string) string {
	return ctV1BasePath + req
}

type addChainRequest struct {
	Chain []string
}

type addChainResponse struct {
	SctVersion int    `json:sct_version`
	ID         string `json:id`
	Timestamp  string `json:timestamp`
	Extensions string `json:extensions`
	Signature  string `json:signature`
}

func parseBodyAsJSONChain(w http.ResponseWriter, r *http.Request) (addChainRequest, error) {
	body, err := ioutil.ReadAll(r.Body)

	if err != nil {
		if glog.V(logVerboseLevel) {
			glog.Info("Failed to read request body")
		}
		sendHttpError(w, http.StatusBadRequest, err)
		return addChainRequest{}, err
	}

	var req addChainRequest
	if err := json.Unmarshal(body, &req); err != nil {
		if glog.V(logVerboseLevel) {
			glog.Infof("Failed to unmarshal: %s", body)
		}
		sendHttpError(w, http.StatusBadRequest, fmt.Errorf("Unmarshal failed with %v on %s", err, body))
		return addChainRequest{}, err
	}

	// The cert chain is not allowed to be empty. We'll defer other validation for later
	if len(req.Chain) == 0 {
		if glog.V(logVerboseLevel) {
			glog.Infof("Request chain is empty: %s", body)
		}
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

// All the handlers are wrapped so they have access to the RPC client
// TODO(Martin2112): Doesn't properly handle duplicate submissions yet but the backend
// needs this to be implemented before we can do it here
func wrappedAddChainHandler(c CTRequestHandlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		validPath := verifyAddChain(addChainRequest, w, *c.trustedRoots, false)

		if validPath == nil {
			// Chain rejected by verify. HTTP status code was already set
			return
		}

		// Build up the SCT and MerkleTreeLeaf. The SCT will be returned to the client and
		// the leaf will become part of the data sent to the backend.
		// TODO(Martin2112): Fixup the leaf -> backend when we have implemented missing serializers
		_, sct, sctBytes, err := buildAndSerializeSCT(addChainRequest, w, c.logKeyManager, validPath[0], c.timeSource)

		if err != nil {
			glog.Warningf("Failed to create / serialize SCT: %v %v", sct, err)
			sendHttpError(w, http.StatusInternalServerError, err)
			return
		}

		// Inputs validated, pass the request on to the back end
		leafHash := sha256.Sum256(validPath[0].Raw)
		leafProto := trillian.LeafProto{LeafHash: leafHash[:], LeafData: validPath[0].Raw, ExtraData: sctBytes}

		request := trillian.QueueLeavesRequest{LogId: c.logID, Leaves: []*trillian.LeafProto{&leafProto}}

		ctx, _ := context.WithDeadline(context.Background(), getRPCDeadlineTime(c))

		response, err := c.rpcClient.QueueLeaves(ctx, &request)

		if err != nil || !rpcStatusOK(response.GetStatus()) {
			// Request failed on backend, doesn't account for bad request etc. yet
			sendHttpError(w, http.StatusInternalServerError, err)
			return
		}

		logID, signature, err := marshalLogIDAndSignatureForResponse(sct, c.logKeyManager)

		if err != nil {
			glog.Warningf("failed to marshal for response: %v", err)
			sendHttpError(w, http.StatusInternalServerError, err)
			return
		}

		// Success. We can now build and marshal the JSON response and write it out
		resp := addChainResponse{
			SctVersion: int(sct.SCTVersion),
			Timestamp:  strconv.FormatUint(sct.Timestamp, 10),
			ID:         base64.StdEncoding.EncodeToString(logID[:]),
			Extensions: "",
			Signature:  signature}

		w.Header().Set(contentTypeHeader, contentTypeJSON)
		jsonData, err := json.Marshal(&resp)

		if err != nil {
			glog.Warningf("Failed to marshal add-chain resp: %v", resp)
			sendHttpError(w, http.StatusInternalServerError, err)
			return
		}

		_, err = w.Write(jsonData)

		if err != nil {
			glog.Warningf("Failed to write add-chain resp: %v", resp)
			// Probably too late for this as headers might have been written but we don't know for sure
			sendHttpError(w, http.StatusInternalServerError, err)
			return
		}
	}
}

func wrappedAddPreChainHandler(rpcClient trillian.TrillianLogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !enforceMethod(w, r, httpMethodPost) {
			return
		}

		_, err := parseBodyAsJSONChain(w, r)

		if err != nil {
			return
		}

		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
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

func wrappedGetEntriesHandler(rpcClient trillian.TrillianLogClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !enforceMethod(w, r, httpMethodGet) {
			return
		}

		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
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
	http.HandleFunc(pathFor("add-pre-chain"), wrappedAddPreChainHandler(c.rpcClient))
	http.HandleFunc(pathFor("get-sth"), wrappedGetSTHHandler(c.rpcClient))
	http.HandleFunc(pathFor("get-sth-consistency"), wrappedGetSTHConsistencyHandler(c.rpcClient))
	http.HandleFunc(pathFor("get-proof-by-hash"), wrappedGetProofByHashHandler(c.rpcClient))
	http.HandleFunc(pathFor("get-entries"), wrappedGetEntriesHandler(c.rpcClient))
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

	// The type of the leaf must match the one the handler expects
	if isPrecert != expectingPrecert || err != nil {
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

// buildAndSerializeSCT is used by add-chain and add-pre-chain. It constructs an SCT and
// signs it with our key. It returns the SCT as an object and serialized as bytes to be passed
// to the log backend.
// TODO(Martin2112): Update to params when add-pre-chain implemented
func buildAndSerializeSCT(req addChainRequest, w http.ResponseWriter, km crypto.KeyManager, cert *x509.Certificate, timeSource util.TimeSource) (ct.MerkleTreeLeaf, ct.SignedCertificateTimestamp, []byte, error) {
	merkleTreeLeaf, sct, err := SignV1SCTForCertificate(km, cert, timeSource.Now())

	if err != nil {
		// Probably a server failure, though it could be bad data like an invalid cert
		glog.Warningf("Failed to create SCT for cert: %v", req)
		sendHttpError(w, http.StatusInternalServerError, err)
		return ct.MerkleTreeLeaf{}, ct.SignedCertificateTimestamp{}, nil, err
	}

	sctBytes, err := ct.SerializeSCT(sct)

	if err != nil {
		glog.Warningf("Failed to serialize SCT: %v", sct)
		sendHttpError(w, http.StatusInternalServerError, err)
		return ct.MerkleTreeLeaf{}, ct.SignedCertificateTimestamp{}, nil, err
	}

	return merkleTreeLeaf, sct, sctBytes, nil
}

// marshalLogIDAndSignatureForResponse is used by add-chain and add-pre-chain. It formats the
// signature and log id ready to send to the client.
func marshalLogIDAndSignatureForResponse(sct ct.SignedCertificateTimestamp, km crypto.KeyManager) ([sha256.Size]byte, string, error) {
	logID, err := GetCTLogID(km)

	if err != nil {
		return [32]byte{}, "", fmt.Errorf("failed to marshal logID: %v %v", logID, err)
	}

	signature, err := sct.Signature.Base64String()

	if err != nil {
		return [32]byte{}, "", fmt.Errorf("failed to marshal signature: %v %v", sct.Signature, err)
	}

	return logID, signature, nil
}
