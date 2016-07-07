package ct_fe

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

var postHandlers = []http.HandlerFunc{ addChainHandler, addPreChainHandler }
var postPaths = []string{ "add-chain", "add-pre-chain"}
var getHandlers = []http.HandlerFunc{ getSthHandler, getSthConsistencyHandler, getProofByHashHandler, getEntriesHandler, getRootsHandler, getEntryAndProofHandler }
var getPaths = []string{ "get-sth", "get-sth-consistency", "get-proof-by-hash", "get-entries", "get-roots", "get-entry-and-proof" }

func TestPostHandlersOnlyAcceptPost(t *testing.T) {
	// Anything in the post handler list should only accept POST
	for i, handler := range postHandlers {
		s := httptest.NewServer(handler)
		defer s.Close()
		resp, err := http.Get(s.URL + "/ct/v1/" + postPaths[i])

		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, "Wrong status code for GET to POST handler")

		resp, err = http.Post(s.URL + "/ct/v1/" + postPaths[i], "application/json", nil)

		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Wrong status code for POST to POST handler")
	}
}

func TestGetHandlersOnlyAcceptGet(t *testing.T) {
	// Anything in the get handler list should only accept GET
	for i, handler := range getHandlers {
		s := httptest.NewServer(handler)
		defer s.Close()
		resp, err := http.Get(s.URL + "/ct/v1/" + getPaths[i])

		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusNotImplemented, resp.StatusCode, "Wrong status code for GET to GET handler")

		resp, err = http.Post(s.URL + "/ct/v1/" + getPaths[i], "application/json", nil)

		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, "Wrong status code for POST to GET handler")
	}
}
