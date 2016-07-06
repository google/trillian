package ct_fe

import (
	"encoding/json"
	"net/http"
	"io/ioutil"
)

const (
	// All RFC6962 requests start with this base path
	ctV1BasePath string = "/ct/v1/"
	// You'd think these would be defined in some library but if so I haven't found it yet
	httpMethodPost = "POST"
	httpMethodGet = "GET"
)

func pathFor(req string) string {
	return ctV1BasePath + req
}

type addChainRequest struct {
	chain []string
}

func parseBodyAsJSONChain(w http.ResponseWriter, r *http.Request) (addChainRequest, error) {
	body, err := ioutil.ReadAll(r.Body)

	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return addChainRequest{}, err
	}

	var req addChainRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return addChainRequest{}, err
	}

	return req, nil
}

// enforceMethod checks that the request method is the one we expect. If it returns
// false then the http status has been set appropriately
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

func RegisterCTHandlers() {
	http.HandleFunc(pathFor("add-chain"), addChainHandler)
	http.HandleFunc(pathFor("add-pre-chain"), addPreChainHandler)
	http.HandleFunc(pathFor("get-sth"), getSthHandler)
	http.HandleFunc(pathFor("get-sth-consistency"), getSthConsistencyHandler)
	http.HandleFunc(pathFor("get-proof-by-hash"), getProofByHashHandler)
	http.HandleFunc(pathFor("get-entries"), getEntriesHandler)
	http.HandleFunc(pathFor("get-roots"), getRootsHandler)
	http.HandleFunc(pathFor("get-entry-and-proof"), getEntryAndProofHandler)
}
