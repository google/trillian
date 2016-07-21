package ct

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/google/certificate-transparency/go"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
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
	jsonMapKeyCertificates string = "certificates"
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
}

// NewCTRequestHandlers creates a new instance of CTRequestHandlers. They must still
// be registered by calling RegisterCTHandlers()
func NewCTRequestHandlers(logID int64, trustedRoots *PEMCertPool, rpcClient trillian.TrillianLogClient, km crypto.KeyManager, rpcDeadline time.Duration) *CTRequestHandlers {
	return &CTRequestHandlers{logID, trustedRoots, rpcClient, km, rpcDeadline}
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
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return addChainRequest{}, err
	}

	var req addChainRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest)+": "+err.Error(), http.StatusBadRequest)
		return addChainRequest{}, err
	}

	// The cert chain is not allowed to be empty. We'll defer other validation for later
	if len(req.Chain) == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest)+": cert chain cannot be empty", http.StatusBadRequest)
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
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
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
			return
		}

		addChainRequest, err := parseBodyAsJSONChain(w, r)

		if err != nil {
			return
		}

		// We already checked that the chain is not empty so can move on to verification
		validPath, err := ValidateChain(addChainRequest.Chain, *c.trustedRoots)

		if err != nil {
			// We rejected it because the cert failed checks or we could not find a path to a root etc.
			// Lots of possible causes for errors
			glog.Warningf("Chain failed to verify: %v", addChainRequest)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		isPrecert, err := IsPrecertificate(validPath[0])

		if isPrecert || err != nil {
			// This handler doesn't expect to see precerts and reject if we can't tell
			glog.Warningf("Precert (or cert with invalid CT ext) submitted to add chain: %v", addChainRequest)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		sct, err := SignV1SCTForCertificate(c.logKeyManager, validPath[0], time.Now())

		if err != nil {
			// Probably a server failure, though it could be bad data like an invalid cert
			glog.Warningf("Failed to create SCT for cert: %v", addChainRequest)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		sctBytes, err := ct.SerializeSCT(sct)
		leafHash := sha256.Sum256(validPath[0].Raw)

		if err != nil {
			glog.Warningf("Failed to serialize SCT: %v", sct)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		// Inputs validated, pass the request on to the back end
		leafProto := trillian.LeafProto{LeafHash: leafHash[:], LeafData: validPath[0].Raw, ExtraData: sctBytes}

		request := trillian.QueueLeavesRequest{LogId: c.logID, Leaves: []*trillian.LeafProto{&leafProto}}

		ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(c.rpcDeadline))

		response, err := c.rpcClient.QueueLeaves(ctx, &request)

		if err != nil || response.Status.StatusCode != trillian.TrillianApiStatusCode_OK {
			// Request failed on backend, doesn't account for bad request etc. yet
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		logID, err := GetCTLogID(c.logKeyManager)

		if err != nil {
			glog.Warningf("Failed to marshal log id: %v", sct.LogID)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		signature, err := sct.Signature.Base64String()

		if err != nil {
			glog.Warningf("Failed to marshal signature: %v", sct.Signature)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		// Success. We can now build and marshal the response
		resp := addChainResponse{
			SctVersion: int(sct.SCTVersion),
			ID:         base64.StdEncoding.EncodeToString(logID[:]),
			Extensions: "",
			Signature:  signature}

		err = json.NewEncoder(w).Encode(&resp)

		if err != nil {
			glog.Warningf("Failed to marshal add-chain resp: %v", resp)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		http.Error(w, http.StatusText(http.StatusOK), http.StatusOK)
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
		jsonMap[jsonMapKeyCertificates] = trustedRoots.RawCertificates()
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
