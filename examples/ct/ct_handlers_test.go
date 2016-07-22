package ct

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/examples/ct/testonly"
	"github.com/google/trillian/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/net/context"
)

// Arbitrary time for use in tests
var fakeTime = time.Date(2016, 7, 22, 11, 01, 13, 0, time.UTC)

// The deadline should be the above bumped by 500ms
var fakeDeadlineTime = time.Date(2016, 7, 22, 11, 01, 13, 500*1000*1000, time.UTC)
var fakeTimeSource = util.FakeTimeSource{fakeTime}

type jsonChain struct {
	Chain []string `json:chain`
}

const caCertB64 string = `MIIC0DCCAjmgAwIBAgIBADANBgkqhkiG9w0BAQUFADBVMQswCQYDVQQGEwJHQjEk
MCIGA1UEChMbQ2VydGlmaWNhdGUgVHJhbnNwYXJlbmN5IENBMQ4wDAYDVQQIEwVX
YWxlczEQMA4GA1UEBxMHRXJ3IFdlbjAeFw0xMjA2MDEwMDAwMDBaFw0yMjA2MDEw
MDAwMDBaMFUxCzAJBgNVBAYTAkdCMSQwIgYDVQQKExtDZXJ0aWZpY2F0ZSBUcmFu
c3BhcmVuY3kgQ0ExDjAMBgNVBAgTBVdhbGVzMRAwDgYDVQQHEwdFcncgV2VuMIGf
MA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDVimhTYhCicRmTbneDIRgcKkATxtB7
jHbrkVfT0PtLO1FuzsvRyY2RxS90P6tjXVUJnNE6uvMa5UFEJFGnTHgW8iQ8+EjP
KDHM5nugSlojgZ88ujfmJNnDvbKZuDnd/iYx0ss6hPx7srXFL8/BT/9Ab1zURmnL
svfP34b7arnRsQIDAQABo4GvMIGsMB0GA1UdDgQWBBRfnYgNyHPmVNT4DdjmsMEk
tEfDVTB9BgNVHSMEdjB0gBRfnYgNyHPmVNT4DdjmsMEktEfDVaFZpFcwVTELMAkG
A1UEBhMCR0IxJDAiBgNVBAoTG0NlcnRpZmljYXRlIFRyYW5zcGFyZW5jeSBDQTEO
MAwGA1UECBMFV2FsZXMxEDAOBgNVBAcTB0VydyBXZW6CAQAwDAYDVR0TBAUwAwEB
/zANBgkqhkiG9w0BAQUFAAOBgQAGCMxKbWTyIF4UbASydvkrDvqUpdryOvw4BmBt
OZDQoeojPUApV2lGOwRmYef6HReZFSCa6i4Kd1F2QRIn18ADB8dHDmFYT9czQiRy
f1HWkLxHqd81TbD26yWVXeGJPE3VICskovPkQNJ0tU4b03YmnKliibduyqQQkOFP
OwqULg==`

const intermediateCertB64 string = `MIIC3TCCAkagAwIBAgIBCTANBgkqhkiG9w0BAQUFADBVMQswCQYDVQQGEwJHQjEk
MCIGA1UEChMbQ2VydGlmaWNhdGUgVHJhbnNwYXJlbmN5IENBMQ4wDAYDVQQIEwVX
YWxlczEQMA4GA1UEBxMHRXJ3IFdlbjAeFw0xMjA2MDEwMDAwMDBaFw0yMjA2MDEw
MDAwMDBaMGIxCzAJBgNVBAYTAkdCMTEwLwYDVQQKEyhDZXJ0aWZpY2F0ZSBUcmFu
c3BhcmVuY3kgSW50ZXJtZWRpYXRlIENBMQ4wDAYDVQQIEwVXYWxlczEQMA4GA1UE
BxMHRXJ3IFdlbjCBnzANBgkqhkiG9w0BAQEFAAOBjQAwgYkCgYEA12pnjRFvUi5V
/4IckGQlCLcHSxTXcRWQZPeSfv3tuHE1oTZe594Yy9XOhl+GDHj0M7TQ09NAdwLn
o+9UKx3+m7qnzflNxZdfxyn4bxBfOBskNTXPnIAPXKeAwdPIRADuZdFu6c9S24rf
/lD1xJM1CyGQv1DVvDbzysWo2q6SzYsCAwEAAaOBrzCBrDAdBgNVHQ4EFgQUllUI
BQJ4R56Hc3ZBMbwUOkfiKaswfQYDVR0jBHYwdIAUX52IDchz5lTU+A3Y5rDBJLRH
w1WhWaRXMFUxCzAJBgNVBAYTAkdCMSQwIgYDVQQKExtDZXJ0aWZpY2F0ZSBUcmFu
c3BhcmVuY3kgQ0ExDjAMBgNVBAgTBVdhbGVzMRAwDgYDVQQHEwdFcncgV2VuggEA
MAwGA1UdEwQFMAMBAf8wDQYJKoZIhvcNAQEFBQADgYEAIgbascZrcdzglcP2qi73
LPd2G+er1/w5wxpM/hvZbWc0yoLyLd5aDIu73YJde28+dhKtjbMAp+IRaYhgIyYi
hMOqXSGR79oQv5I103s6KjQNWUGblKSFZvP6w82LU9Wk6YJw6tKXsHIQ+c5KITix
iBEUO5P6TnqH3TfhOF8sKQg=`

const caAndIntermediateCertsPEM string = "-----BEGIN CERTIFICATE-----\n" + caCertB64 + "\n-----END CERTIFICATE-----\n" +
	"\n-----BEGIN CERTIFICATE-----\n" + intermediateCertB64 + "\n-----END CERTIFICATE-----\n"

type handlerAndPath struct {
	path    string
	handler http.HandlerFunc
}

func allGetHandlersForTest(trustedRoots *PEMCertPool, client trillian.TrillianLogClient) []handlerAndPath {
	return []handlerAndPath{
		{"get-sth", wrappedGetSTHHandler(client)},
		{"get-sth-consistency", wrappedGetSTHConsistencyHandler(client)},
		{"get-proof-by-hash", wrappedGetProofByHashHandler(client)},
		{"get-entries", wrappedGetEntriesHandler(client)},
		{"get-roots", wrappedGetRootsHandler(trustedRoots, client)},
		{"get-entry-and-proof", wrappedGetEntryAndProofHandler(client)}}
}

func allPostHandlersForTest(client trillian.TrillianLogClient) []handlerAndPath {
	pool := NewPEMCertPool()
	ok := pool.AppendCertsFromPEM([]byte(testonly.FakeCACertPem))

	if !ok {
		glog.Fatal("Failed to load cert pool")
	}

	return []handlerAndPath{
		{"add-chain", wrappedAddChainHandler(CTRequestHandlers{rpcClient: client, trustedRoots: pool})},
		{"add-pre-chain", wrappedAddPreChainHandler(client)}}
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

		resp, err = http.Post(s.URL+"/ct/v1/"+hp.path, "application/json", nil)

		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Wrong status code for POST to POST handler")
	}
}

func TestGetHandlersOnlyAcceptGet(t *testing.T) {
	client := new(trillian.MockTrillianLogClient)
	pool := NewPEMCertPool()

	// Anything in the get handler list should only accept GET
	for _, hp := range allGetHandlersForTest(pool, client) {
		s := httptest.NewServer(hp.handler)
		defer s.Close()
		resp, err := http.Get(s.URL + "/ct/v1/" + hp.path)

		if err != nil {
			t.Fatal(err)
		}

		// TODO(Martin2112): Remove not implemented from test when all the handlers have been written
		assert.True(t, resp.StatusCode == http.StatusNotImplemented || resp.StatusCode == http.StatusOK, "Wrong status code for GET to GET handler")

		resp, err = http.Post(s.URL+"/ct/v1/"+hp.path, "application/json", nil)

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

		resp, err := http.Post(s.URL+"/ct/v1/"+hp.path, "application/json", strings.NewReader(""))

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

		resp, err := http.Post(s.URL+"/ct/v1/"+hp.path, "application/json", strings.NewReader("{ !£$%^& not valid json "))

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

		resp, err := http.Post(s.URL+"/ct/v1/"+hp.path, "application/json", strings.NewReader(`{ "chain": [] }`))

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

		resp, err := http.Post(s.URL+"/ct/v1/"+hp.path, "application/json", strings.NewReader(`{ "chain": [ "test" ] }`))

		if err != nil {
			t.Fatal(err)
		}

		// TODO(Martin2112): Remove not implemented from test when all the handlers have been written
		assert.True(t, resp.StatusCode == http.StatusNotImplemented || resp.StatusCode == http.StatusBadRequest, "Wrong status code for GET to GET handler")
	}
}

func TestGetRoots(t *testing.T) {
	client := new(trillian.MockTrillianLogClient)
	roots := loadCertsIntoPoolOrDie(t, []string{caAndIntermediateCertsPEM})
	handler := wrappedGetRootsHandler(roots, client)

	req, err := http.NewRequest("GET", "http://example.com/ct/v1/get-roots", nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	handler(w, req)

	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, http.StatusOK, w.Code, "Expected HTTP OK for get-roots")

	var parsedJson map[string][]string
	if err := json.Unmarshal(w.Body.Bytes(), &parsedJson); err != nil {
		t.Fatalf("Failed to unmarshal json response: %s", w.Body.Bytes())
	}
	assert.Equal(t, 1, len(parsedJson), "Expected one entry in json map")
	certs := parsedJson[jsonMapKeyCertificates]
	assert.Equal(t, 2, len(certs), "Expected two root certs: %v", certs)
	assert.Equal(t, strings.Replace(caCertB64, "\n", "", -1), certs[0], "First root cert mismatched")
	assert.Equal(t, strings.Replace(intermediateCertB64, "\n", "", -1), certs[1], "Second root cert mismatched")

	client.AssertExpectations(t)
}

// This uses the fake CA as trusted root and submits a chain of just a leaf which should be rejected
// because there's no complete path
func TestAddChainMissingIntermediate(t *testing.T) {
	km := new(crypto.MockKeyManager)
	client := new(trillian.MockTrillianLogClient)

	roots := loadCertsIntoPoolOrDie(t, []string{testonly.FakeCACertPem})
	reqHandlers := CTRequestHandlers{0x42, roots, client, km, time.Millisecond * 500, fakeTimeSource}

	pool := loadCertsIntoPoolOrDie(t, []string{testonly.LeafSignedByFakeIntermediateCertPem})
	chain := createJsonChain(t, *pool)

	recorder := makeAddChainRequest(t, reqHandlers, chain)

	assert.Equal(t, http.StatusBadRequest, recorder.Code, "expected HTTP BadRequest for incomplete add-chain: %v", recorder.Body)
	km.AssertExpectations(t)
	client.AssertExpectations(t)
}

// This uses the fake CA as trusted root and submits a chain leaf -> fake intermediate, which
// should be accepted
func TestAddChain(t *testing.T) {
	toSign := []byte{0xd1, 0x66, 0x49, 0xc7, 0xbb, 0x48, 0xe7, 0x32, 0xa9, 0x71, 0xc3, 0x1b, 0x26, 0xf6, 0x5c, 0x26, 0x85, 0xd3, 0xc, 0xed, 0x22, 0x48, 0xc4, 0xd4, 0xdb, 0xaa, 0xee, 0x9d, 0x44, 0xf4, 0xc1, 0x6f}
	km := setupMockKeyManager(toSign)
	client := new(trillian.MockTrillianLogClient)

	roots := loadCertsIntoPoolOrDie(t, []string{testonly.FakeCACertPem})
	reqHandlers := CTRequestHandlers{0x42, roots, client, km, time.Millisecond * 500, fakeTimeSource}

	pool := loadCertsIntoPoolOrDie(t, []string{testonly.LeafSignedByFakeIntermediateCertPem, testonly.FakeIntermediateCertPem})
	chain := createJsonChain(t, *pool)

	leaves := leafProtosForCert(t, km, pool.RawCertificates()[0])

	client.On("QueueLeaves", mock.MatchedBy(deadlineMatcher), &trillian.QueueLeavesRequest{LogId: 0x42, Leaves: leaves}, mock.Anything /* []grpc.CallOption */).Return(&trillian.QueueLeavesResponse{}, nil)

	recorder := makeAddChainRequest(t, reqHandlers, chain)

	assert.Equal(t, http.StatusOK, recorder.Code, "expected HTTP OK for valid add-chain: %v", recorder.Body)
	km.AssertExpectations(t)
	client.AssertExpectations(t)

	// Roundtrip the response and make sure it's sensible
	var resp addChainResponse
	err := json.NewDecoder(recorder.Body).Decode(&resp)

	assert.NoError(t, err, "failed to unmarshal json: %v", recorder.Body.Bytes())

	assert.Equal(t, ct.V1, ct.Version(resp.SctVersion))
	assert.Equal(t, ctMockLogID, resp.ID)
	assert.Equal(t, "1469185273000000", resp.Timestamp)
	assert.Equal(t, "BAEABnNpZ25lZA==", resp.Signature)
}

func loadCertsIntoPoolOrDie(t *testing.T, certs []string) *PEMCertPool {
	pool := NewPEMCertPool()

	for _, cert := range certs {
		ok := pool.AppendCertsFromPEM([]byte(cert))

		if !ok {
			t.Fatalf("couldn't parse test certs: %v", certs)
		}
	}

	return pool
}

func createJsonChain(t *testing.T, p PEMCertPool) io.Reader {
	var chain jsonChain

	for _, rawCert := range p.RawCertificates() {
		b64 := base64.StdEncoding.EncodeToString(rawCert.Raw)
		chain.Chain = append(chain.Chain, b64)
	}

	var buffer bytes.Buffer
	// It's tempting to avoid creating and flushing the intermediate writer but it doesn't work
	writer := bufio.NewWriter(&buffer)
	err := json.NewEncoder(writer).Encode(&chain)
	writer.Flush()

	if err != nil {
		t.Fatalf("failed to create test json: %v", chain)
	}

	return bufio.NewReader(&buffer)
}

func leafProtosForCert(t *testing.T, km crypto.KeyManager, cert *x509.Certificate) []*trillian.LeafProto {
	leafHash := sha256.Sum256(cert.Raw)
	sct, err := SignV1SCTForCertificate(km, cert, fakeTime)

	if err != nil {
		t.Fatalf("Failed to sign test SCT: %v", err)
	}

	extraData, err := ct.SerializeSCT(sct)

	if err != nil {
		t.Fatalf("Failed to serialize test SCT: %v", err)
	}

	return []*trillian.LeafProto{&trillian.LeafProto{LeafHash: leafHash[:], LeafData: cert.Raw, ExtraData: extraData}}
}

func deadlineMatcher(other context.Context) bool {
	deadlineTime, ok := other.Deadline()

	if !ok {
		return false // we never make RPC calls without a deadline set
	}

	return deadlineTime == fakeDeadlineTime
}

func makeAddChainRequest(t *testing.T, reqHandlers CTRequestHandlers, body io.Reader) *httptest.ResponseRecorder {
	handler := wrappedAddChainHandler(reqHandlers)

	req, err := http.NewRequest("POST", "http://example.com/ct/v1/add-chain", body)
	if err != nil {
		t.Fatalf("test request setup failed: %v", err)
	}

	w := httptest.NewRecorder()
	handler(w, req)

	if err != nil {
		t.Fatalf("error from handler: %v", err)
	}

	return w
}