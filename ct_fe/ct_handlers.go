package ct_fe

import "net/http"

const (
	// All RFC6962 requests start with this base path
	ctV1BasePath string = "/ct/v1/"
	// You'd think these would be defined in some library but if so I haven't found it yet
	httpMethodPost = "POST"
	httpMethodGet = "GET"
)

func pathFor(req string) {
	return ctV1BasePath + req
}

// enforceMethod checks that the request method is the one we expect. If it returns
// false then the http status has been set appropriately
func enforceMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return false
	}

	return true
}

func addChainHandler(w http.ResponseWriter, r *http.Request) {
	if !enforceMethod(w, r, httpMethodPost) {
		return
	}

	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

func addPreChainHandler(w http.ResponseWriter, r *http.Request) {
	if !enforceMethod(w, r, httpMethodPost) {
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
	http.Handle(pathFor("add-chain"), addChainHandler)
	http.Handle(pathFor("add-pre-chain"), addPreChainHandler)
	http.Handle(pathFor("get-sth"), getSthHandler)
	http.Handle(pathFor("get-sth-consistency"), getSthConsistencyHandler)
	http.Handle(pathFor("get-proof-by-hash"), getProofByHashHandler)
	http.Handle(pathFor("get-entries"), getEntriesHandler)
	http.Handle(pathFor("get-roots"), getRootsHandler)
	http.Handle(pathFor("get-entry-and-proof"), getEntryAndProofHandler)
}
