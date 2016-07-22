package ct

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"
	"github.com/google/trillian"
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
	trustedRoots *PEMCertPool
	rpcClient    trillian.TrillianLogClient
}

// NewCTRequestHandlers creates a new instance of CTRequestHandlers. They must still
// be registered by calling RegisterCTHandlers()
func NewCTRequestHandlers(trustedRoots *PEMCertPool, rpcClient trillian.TrillianLogClient) *CTRequestHandlers {
	return &CTRequestHandlers{trustedRoots, rpcClient}
}

func pathFor(req string) string {
	return ctV1BasePath + req
}

type addChainRequest struct {
	Chain []string
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
func wrappedAddChainHandler(rpcClient trillian.TrillianLogClient) http.HandlerFunc {
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
	http.HandleFunc(pathFor("add-chain"), wrappedAddChainHandler(c.rpcClient))
	http.HandleFunc(pathFor("add-pre-chain"), wrappedAddPreChainHandler(c.rpcClient))
	http.HandleFunc(pathFor("get-sth"), wrappedGetSTHHandler(c.rpcClient))
	http.HandleFunc(pathFor("get-sth-consistency"), wrappedGetSTHConsistencyHandler(c.rpcClient))
	http.HandleFunc(pathFor("get-proof-by-hash"), wrappedGetProofByHashHandler(c.rpcClient))
	http.HandleFunc(pathFor("get-entries"), wrappedGetEntriesHandler(c.rpcClient))
	http.HandleFunc(pathFor("get-roots"), wrappedGetRootsHandler(c.trustedRoots, c.rpcClient))
	http.HandleFunc(pathFor("get-entry-and-proof"), wrappedGetEntryAndProofHandler(c.rpcClient))
}
