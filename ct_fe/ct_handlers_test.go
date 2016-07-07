package ct_fe

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/google/trillian"
)

type handlerAndPath struct {
	path string
	handler http.HandlerFunc
}

func allGetHandlersForTest(client trillian.TrillianLogClient) []handlerAndPath {
	return []handlerAndPath{
		{ "get-sth", wrappedGetSthHandler(client) },
		{ "get-sth-consistency", wrappedGetSthConsistencyHandler(client) },
		{ "get-proof-by-hash", wrappedGetProofByHashHandler(client) },
		{ "get-entries", wrappedGetEntriesHandler(client) },
		{ "get-roots", wrappedGetRootsHandler(client) },
		{ "get-entry-and-proof", wrappedGetEntryAndProofHandler(client) }}
}

func allPostHandlersForTest(client trillian.TrillianLogClient) []handlerAndPath {
	return []handlerAndPath{
		{ "add-chain", wrappedAddChainHandler(client) },
		{ "add-pre-chain", wrappedAddPreChainHandler(client) }}
}

func TestPostHandlersOnlyAcceptPost(t *testing.T) {
	client := new(trillian.MockTrillianLogClient)

	// Anything in the post handler list should only accept POST
	for _, hp := range allPostHandlersForTest(client) {
		s := httptest.NewServer(hp.handler)
		defer s.Close()
		resp, err := http.Get(s.URL + "/ct/v1/" + hp.path)

		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, "Wrong status code for GET to POST handler")

		resp, err = http.Post(s.URL + "/ct/v1/" + hp.path, "application/json", nil)

		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Wrong status code for POST to POST handler")
	}
}

func TestGetHandlersOnlyAcceptGet(t *testing.T) {
	client := new(trillian.MockTrillianLogClient)

	// Anything in the get handler list should only accept GET
	for _, hp := range allGetHandlersForTest(client) {
		s := httptest.NewServer(hp.handler)
		defer s.Close()
		resp, err := http.Get(s.URL + "/ct/v1/" + hp.path)

		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusNotImplemented, resp.StatusCode, "Wrong status code for GET to GET handler")

		resp, err = http.Post(s.URL + "/ct/v1/" + hp.path, "application/json", nil)

		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, "Wrong status code for POST to GET handler")
	}
}

func TestPostHandlersRejectEmptyJson(t *testing.T) {
	client := new(trillian.MockTrillianLogClient)

	for _, hp := range allPostHandlersForTest(client) {
		s := httptest.NewServer(hp.handler)
		defer s.Close()

		resp, err := http.Post(s.URL + "/ct/v1/" + hp.path, "application/json", strings.NewReader(""))

		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Wrong status code for empty JSON body")
	}
}

func TestPostHandlersRejectMalformedJson(t *testing.T) {
	client := new(trillian.MockTrillianLogClient)

	for _, hp := range allPostHandlersForTest(client) {
		s := httptest.NewServer(hp.handler)
		defer s.Close()

		resp, err := http.Post(s.URL + "/ct/v1/" + hp.path, "application/json", strings.NewReader("{ !£$%^& not valid json "))

		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Wrong status code for invalid JSON body")
	}
}

func TestPostHandlersRejectEmptyCertChain(t *testing.T) {
	client := new(trillian.MockTrillianLogClient)

	for _, hp := range allPostHandlersForTest(client) {
		s := httptest.NewServer(hp.handler)
		defer s.Close()

		resp, err := http.Post(s.URL + "/ct/v1/" + hp.path, "application/json", strings.NewReader(`{ "chain": [] }`))

		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Wrong status code for empty chain in JSON body")
	}
}

func TestPostHandlersAcceptNonEmptyCertChain(t *testing.T) {
	client := new(trillian.MockTrillianLogClient)

	for _, hp := range allPostHandlersForTest(client) {
		s := httptest.NewServer(hp.handler)
		defer s.Close()

		resp, err := http.Post(s.URL + "/ct/v1/" + hp.path, "application/json", strings.NewReader(`{ "chain": [ "test" ] }`))

		if err != nil {
			t.Fatal(err)
		}

		// For now they return not implemented as the handler is a stub
		assert.Equal(t, http.StatusNotImplemented, resp.StatusCode, "Wrong status code for non empty chain in body")
	}
}