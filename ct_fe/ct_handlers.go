package ct_fe

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"

	"github.com/google/trillian"
)

const (
	// All RFC6962 requests start with this base path
	ctV1BasePath string = "/ct/v1/"
	// You'd think these would be defined in some library but if so I haven't found it yet
	httpMethodPost = "POST"
	httpMethodGet = "GET"
)

type CtRequestHandlers struct {
	rpcClient trillian.TrillianLogClient
}

func NewCtRequestHandlers(rpcClient trillian.TrillianLogClient) *CtRequestHandlers {
	return &CtRequestHandlers{rpcClient}
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
		http.Error(w, http.StatusText(http.StatusBadRequest) + ": " + err.Error(), http.StatusBadRequest)
		return addChainRequest{}, err
	}

	// The cert chain is not allowed to be empty. We'll defer other validation for later
	if len(req.Chain) == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest) + ": cert chain cannot be empty", http.StatusBadRequest)
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

func addChainHandler(w http.ResponseWriter, r *http.Request) {
	if !enforceMethod(w, r, httpMethodPost) {
		return
	}

	_, err := parseBodyAsJSONChain(w, r)

	if err != nil {
		return
	}

	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

func addPreChainHandler(w http.ResponseWriter, r *http.Request) {
	if !enforceMethod(w, r, httpMethodPost) {
		return
	}

	_, err := parseBodyAsJSONChain(w, r)

	if err != nil {
		return
	}

	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

func getSthHandler(w http.ResponseWriter, r *http.Request) {
	if !enforceMethod(w, r, httpMethodGet) {
		return
	}

	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

func getSthConsistencyHandler(w http.ResponseWriter, r *http.Request) {
	if !enforceMethod(w, r, httpMethodGet) {
		return
	}

	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

func getProofByHashHandler(w http.ResponseWriter, r *http.Request) {
	if !enforceMethod(w, r, httpMethodGet) {
		return
	}

	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

func getEntriesHandler(w http.ResponseWriter, r *http.Request) {
	if !enforceMethod(w, r, httpMethodGet) {
		return
	}

	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

func getRootsHandler(w http.ResponseWriter, r *http.Request) {
	if !enforceMethod(w, r, httpMethodGet) {
		return
	}

	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

func getEntryAndProofHandler(w http.ResponseWriter, r *http.Request) {
	if !enforceMethod(w, r, httpMethodGet) {
		return
	}

	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

// RegisterCTHandlers registers a HandleFunc for all of the RFC6962 defined methods.
// TODO(Martin2112): This registers on default ServeMux, might need more flexibility?
func (c CtRequestHandlers) RegisterCTHandlers() {
	http.HandleFunc(pathFor("add-chain"), addChainHandler)
	http.HandleFunc(pathFor("add-pre-chain"), addPreChainHandler)
	http.HandleFunc(pathFor("get-sth"), getSthHandler)
	http.HandleFunc(pathFor("get-sth-consistency"), getSthConsistencyHandler)
	http.HandleFunc(pathFor("get-proof-by-hash"), getProofByHashHandler)
	http.HandleFunc(pathFor("get-entries"), getEntriesHandler)
	http.HandleFunc(pathFor("get-roots"), getRootsHandler)
	http.HandleFunc(pathFor("get-entry-and-proof"), getEntryAndProofHandler)
}
