package ct

import (
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
	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/tls"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
)

const (
	// HTTP content type header
	contentTypeHeader string = "Content-Type"
	// MIME content type for JSON
	contentTypeJSON string = "application/json"
	// The name of the JSON response map key in get-roots responses
	jsonMapKeyCertificates string = "certificates"
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

// appHandler holds a LogContext and a handler function that uses it, and is
// an implementation of the http.Handler interface.
type appHandler struct {
	context LogContext
	handler func(context.Context, LogContext, http.ResponseWriter, *http.Request) (int, error)
	name    string
	method  string
}

// ServeHTTP for an appHandler invokes the underlying handler function but
// does additional common error processing.
func (a appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if glog.V(2) {
		glog.Infof("%s: request %v %q => %s", a.context.logPrefix, r.Method, r.URL, a.name)
	}
	if r.Method != a.method {
		glog.Warningf("%s: %s wrong HTTP method: %v", a.context.logPrefix, a.name, r.Method)
		sendHTTPError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed: %s", r.Method))
		return
	}

	// For GET requests all params come as form encoded so we might as well parse them now.
	// POSTs will decode the raw request body as JSON later.
	if r.Method == http.MethodGet {
		if err := r.ParseForm(); err != nil {
			sendHTTPError(w, http.StatusBadRequest, fmt.Errorf("failed to parse form data: %v", err))
			return
		}
	}

	// Many/most of the handlers forward the request on to the Log RPC server; impose a deadline
	// on this onward request.
	ctx, cancel := context.WithDeadline(context.TODO(), getRPCDeadlineTime(a.context))
	defer cancel()

	status, err := a.handler(ctx, a.context, w, r)
	glog.V(2).Infof("%s: %s <= status=%d", a.context.logPrefix, a.name, status)
	if err != nil {
		glog.Warningf("%s: %s handler error: %v", a.context.logPrefix, a.name, err)
		sendHTTPError(w, status, err)
		return
	}

	// Additional check, for consistency the handler must return an error for non-200 status
	if status != http.StatusOK {
		glog.Warningf("%s: %s handler non 200 without error: %d %v", a.context.logPrefix, a.name, status, err)
		sendHTTPError(w, http.StatusInternalServerError, fmt.Errorf("http handler misbehaved, status: %d", status))
		return
	}
}

// LogContext holds information for a specific log instance.
type LogContext struct {
	// logID is the tree ID that identifies this log in node storage
	logID int64
	// logPrefix is a pre-formatted string identifying the log for diagnostics.
	logPrefix string
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

// NewLogContext creates a new instance of LogContext.
func NewLogContext(logID int64, trustedRoots *PEMCertPool, rpcClient trillian.TrillianLogClient, km crypto.KeyManager, rpcDeadline time.Duration, timeSource util.TimeSource) *LogContext {
	return &LogContext{
		logID:         logID,
		logPrefix:     fmt.Sprintf("{%d}", logID),
		trustedRoots:  trustedRoots,
		rpcClient:     rpcClient,
		logKeyManager: km,
		rpcDeadline:   rpcDeadline,
		timeSource:    timeSource}
}

func parseBodyAsJSONChain(c LogContext, r *http.Request) (ct.AddChainRequest, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		glog.V(1).Infof("%s: Failed to read request body: %v", c.logPrefix, err)
		return ct.AddChainRequest{}, err
	}

	var req ct.AddChainRequest
	if err := json.Unmarshal(body, &req); err != nil {
		glog.V(1).Infof("%s: Failed to parse request body: %v", c.logPrefix, err)
		return ct.AddChainRequest{}, err
	}

	// The cert chain is not allowed to be empty. We'll defer other validation for later
	if len(req.Chain) == 0 {
		glog.V(1).Infof("%s: Request chain is empty: %s", c.logPrefix, body)
		return ct.AddChainRequest{}, errors.New("cert chain was empty")
	}

	return req, nil
}

// addChainInternal is called by add-chain and add-pre-chain as the logic involved in
// processing these requests is almost identical
// TODO(Martin2112): Doesn't properly handle duplicate submissions yet but the backend
// needs this to be implemented before we can do it here
func addChainInternal(ctx context.Context, c LogContext, w http.ResponseWriter, r *http.Request, isPrecert bool) (int, error) {
	var signerFn func(crypto.KeyManager, *x509.Certificate, time.Time) (ct.MerkleTreeLeaf, ct.SignedCertificateTimestamp, error)
	var method string
	if isPrecert {
		method = "AddPreChain"
		signerFn = signV1SCTForPrecertificate
	} else {
		method = "AddChain"
		signerFn = signV1SCTForCertificate
	}

	// Check the contents of the request and convert to slice of certificates.
	addChainReq, err := parseBodyAsJSONChain(c, r)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("failed to parse add-chain body: %v", err)
	}
	chain, err := verifyAddChain(c, addChainReq, w, isPrecert)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("failed to verify add-chain contents: %v", err)
	}

	// Build up the SCT and MerkleTreeLeaf. The SCT will be returned to the client and
	// the leaf will become part of the data sent to the backend.
	merkleLeaf, sct, err := signerFn(c.logKeyManager, chain[0], c.timeSource.Now())
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to build SCT and Merkle leaf: %v %v", sct, err)
	}

	// Send the Merkle tree leaf on to the Log server.
	leaf, err := buildLogLeafForAddChain(c, merkleLeaf, chain)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to build LogLeaf: %v", err)
	}
	req := trillian.QueueLeavesRequest{LogId: c.logID, Leaves: []*trillian.LogLeaf{&leaf}}

	if glog.V(2) {
		glog.Infof("%s: %s => grpc.QueueLeaves", c.logPrefix, method)
	}
	rsp, err := c.rpcClient.QueueLeaves(ctx, &req)
	if glog.V(2) {
		glog.Infof("%s: %s <= grpc.QueueLeaves status=%v", c.logPrefix, method, rsp.GetStatus())
	}
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("backend QueueLeaves request failed: %v", err)
	}
	if !rpcStatusOK(rsp.GetStatus()) {
		return http.StatusInternalServerError, fmt.Errorf("backend QueueLeaves request failed, status=%v", rsp.GetStatus())
	}

	// As the Log server has successfully queued up the Merkle tree leaf, we can
	// respond with an SCT.
	err = marshalAndWriteAddChainResponse(sct, c.logKeyManager, w)
	if err != nil {
		// reason is logged and http status is already set
		// TODO(Martin2112): Record failure for monitoring when it's implemented
		return http.StatusInternalServerError, fmt.Errorf("failed to write response: %v", err)
	}
	if glog.V(3) {
		glog.Infof("%s: %s <= SCT", c.logPrefix, method)
	}

	return http.StatusOK, nil
}

func addChain(ctx context.Context, c LogContext, w http.ResponseWriter, r *http.Request) (int, error) {
	return addChainInternal(ctx, c, w, r, false)
}

func addPreChain(ctx context.Context, c LogContext, w http.ResponseWriter, r *http.Request) (int, error) {
	return addChainInternal(ctx, c, w, r, true)
}

func getSTH(ctx context.Context, c LogContext, w http.ResponseWriter, r *http.Request) (int, error) {
	// Forward on to the Log server.
	req := trillian.GetLatestSignedLogRootRequest{LogId: c.logID}
	if glog.V(2) {
		glog.Infof("%s: GetSTH => grpc.GetLatestSignedLogRoot %+v", c.logPrefix, req)
	}
	rsp, err := c.rpcClient.GetLatestSignedLogRoot(ctx, &req)
	if glog.V(2) {
		glog.Infof("%s: GetSTH <= grpc.GetLatestSignedLogRoot status=%v", c.logPrefix, rsp.GetStatus())
	}
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("backend GetLatestSignedLogRoot request failed: %v", err)
	}
	if !rpcStatusOK(rsp.GetStatus()) {
		return http.StatusInternalServerError, fmt.Errorf("backend GetLatestSignedLogRoot request failed, status=%v", rsp.GetStatus())
	}

	// Check over the response.
	slr := rsp.GetSignedLogRoot()
	if slr == nil {
		return http.StatusInternalServerError, fmt.Errorf("no log root returned")
	}
	if glog.V(3) {
		glog.Infof("%s: GetSTH <= slr=%+v", c.logPrefix, slr)
	}
	if treeSize := slr.TreeSize; treeSize < 0 {
		return http.StatusInternalServerError, fmt.Errorf("bad tree size from backend: %d", treeSize)
	}

	if hashSize := len(slr.RootHash); hashSize != sha256.Size {
		return http.StatusInternalServerError, fmt.Errorf("bad hash size from backend expecting: %d got %d", sha256.Size, hashSize)
	}

	// Build the CT STH object, including a signature over its contents.
	sth := ct.SignedTreeHead{
		Version:   ct.V1,
		TreeSize:  uint64(slr.TreeSize),
		Timestamp: uint64(slr.TimestampNanos / 1000 / 1000),
	}
	copy(sth.SHA256RootHash[:], slr.RootHash) // Checked size above.
	err = signV1TreeHead(c.logKeyManager, &sth)
	if err != nil || len(sth.TreeHeadSignature.Signature) == 0 {
		return http.StatusInternalServerError, fmt.Errorf("failed to sign tree head: %v", err)
	}

	// Now build the final result object that will be marshalled to JSON
	jsonRsp := ct.GetSTHResponse{
		TreeSize:       sth.TreeSize,
		SHA256RootHash: sth.SHA256RootHash[:],
		Timestamp:      sth.Timestamp,
	}
	jsonRsp.TreeHeadSignature, err = tls.Marshal(sth.TreeHeadSignature)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to tls.Marshal signature: %v", err)
	}

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	jsonData, err := json.Marshal(&jsonRsp)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to marshal response: %v %v", jsonRsp, err)
	}

	_, err = w.Write(jsonData)
	if err != nil {
		// Probably too late for this as headers might have been written but we don't know for sure
		return http.StatusInternalServerError, fmt.Errorf("failed to write response data: %v", err)
	}

	return http.StatusOK, nil
}

func getSTHConsistency(ctx context.Context, c LogContext, w http.ResponseWriter, r *http.Request) (int, error) {
	first, second, err := parseGetSTHConsistencyRange(r)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("failed to parse consistency range: %v", err)
	}
	req := trillian.GetConsistencyProofRequest{LogId: c.logID, FirstTreeSize: first, SecondTreeSize: second}

	if glog.V(2) {
		glog.Infof("%s: GetSTHConsistency(%d, %d) => grpc.GetConsistencyProof %+v", c.logPrefix, first, second, req)
	}
	rsp, err := c.rpcClient.GetConsistencyProof(ctx, &req)
	if glog.V(2) {
		glog.Infof("%s: GetSTHConsistency <= grpc.GetConsistencyProof status=%v", c.logPrefix, rsp.GetStatus())
	}
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("backend GetConsistencyProof request failed: %v", err)
	}
	if !rpcStatusOK(rsp.GetStatus()) {
		return http.StatusInternalServerError, fmt.Errorf("backend GetConsistencyProof request failed, status=%v", rsp.GetStatus())
	}

	// Additional sanity checks, none of the hashes in the returned path should be empty
	if !checkAuditPath(rsp.Proof.ProofNode) {
		return http.StatusInternalServerError, fmt.Errorf("backend returned invalid proof: %v", rsp.Proof)
	}

	// We got a valid response from the server. Marshal it as JSON and return it to the client
	jsonRsp := ct.GetSTHConsistencyResponse{Consistency: auditPathFromProto(rsp.Proof.ProofNode)}

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	jsonData, err := json.Marshal(&jsonRsp)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-sth-consistency resp: %v because %v", jsonRsp, err)
	}

	_, err = w.Write(jsonData)
	if err != nil {
		// Probably too late for this as headers might have been written but we don't know for sure
		return http.StatusInternalServerError, fmt.Errorf("failed to write get-sth-consistency resp: %v because %v", jsonRsp, err)
	}

	return http.StatusOK, nil
}

func getProofByHash(ctx context.Context, c LogContext, w http.ResponseWriter, r *http.Request) (int, error) {
	// Accept any non empty hash that decodes from base64 and let the backend validate it further
	hash := r.FormValue(getProofParamHash)
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
	req := trillian.GetInclusionProofByHashRequest{
		LogId:           c.logID,
		LeafHash:        leafHash,
		TreeSize:        treeSize,
		OrderBySequence: true,
	}
	rsp, err := c.rpcClient.GetInclusionProofByHash(ctx, &req)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("backend GetInclusionProofByHash request failed: %v", err)
	}
	if !rpcStatusOK(rsp.GetStatus()) {
		return http.StatusInternalServerError, fmt.Errorf("backend GetInclusionProofByHash request failed, status=%v", rsp.GetStatus())
	}

	// Additional sanity checks, none of the hashes in the returned path should be empty
	if !checkAuditPath(rsp.Proof[0].ProofNode) {
		return http.StatusInternalServerError, fmt.Errorf("get-proof-by-hash: backend returned invalid proof: %v", rsp.Proof[0])
	}

	// All checks complete, marshal and return the response
	proofRsp := ct.GetProofByHashResponse{LeafIndex: rsp.Proof[0].LeafIndex, AuditPath: auditPathFromProto(rsp.Proof[0].ProofNode)}

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	jsonData, err := json.Marshal(&proofRsp)
	if err != nil {
		glog.Warningf("%s: Failed to marshal get-proof-by-hash resp: %v", c.logPrefix, proofRsp)
		return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-proof-by-hash resp: %v, error: %v", proofRsp, err)
	}

	_, err = w.Write(jsonData)
	if err != nil {
		// Probably too late for this as headers might have been written but we don't know for sure
		return http.StatusInternalServerError, fmt.Errorf("failed to write get-proof-by-hash resp: %v", proofRsp)
	}

	return http.StatusOK, nil
}

func getEntries(ctx context.Context, c LogContext, w http.ResponseWriter, r *http.Request) (int, error) {
	// The first job is to parse the params and make sure they're sensible. We just make
	// sure the range is valid. We don't do an extra roundtrip to get the current tree
	// size and prefer to let the backend handle this case
	start, end, err := parseGetEntriesRange(r, maxGetEntriesAllowed)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("bad range on get-entries request: %v", err)
	}

	// Now make a request to the backend to get the relevant leaves
	req := trillian.GetLeavesByIndexRequest{
		LogId:     c.logID,
		LeafIndex: buildIndicesForRange(start, end),
	}
	rsp, err := c.rpcClient.GetLeavesByIndex(ctx, &req)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("backend GetLeavesByIndex request failed: %v", err)
	}
	if !rpcStatusOK(rsp.GetStatus()) {
		return http.StatusInternalServerError, fmt.Errorf("backend GetLeavesByIndex request failed, status=%v", rsp.GetStatus())
	}

	// Apply additional checks on the response to make sure we got a contiguous leaf range.
	// It's allowed by the RFC for the backend to truncate the range in cases where the
	// range exceeds the tree size etc. so we could get fewer leaves than we requested but
	// never more and never anything outside the requested range.
	if got, want := len(rsp.Leaves), len(req.LeafIndex); got > want {
		return http.StatusInternalServerError, fmt.Errorf("backend returned too many leaves: %d v %d", got, want)
	}

	if err := isResponseContiguousRange(rsp, start, end); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("backend get-entries range received from backend non contiguous: %v", err)
	}

	// Now we've checked the response and it seems to be valid we need to serialize the
	// leaves in JSON format. Doing a round trip via the leaf deserializer gives us another
	// chance to prevent bad / corrupt data from reaching the client.
	jsonRsp, err := marshalGetEntriesResponse(c, rsp)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to process leaves returned from backend: %v", err)
	}

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	jsonData, err := json.Marshal(&jsonRsp)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-entries resp: %v because: %v", jsonRsp, err)
	}

	_, err = w.Write(jsonData)
	if err != nil {
		// Probably too late for this as headers might have been written but we don't know for sure
		return http.StatusInternalServerError, fmt.Errorf("failed to write get-entries resp: %v because: %v", jsonRsp, err)
	}

	return http.StatusOK, nil
}

func getRoots(ctx context.Context, c LogContext, w http.ResponseWriter, r *http.Request) (int, error) {
	// Pull out the raw certificates from the parsed versions
	rawCerts := make([][]byte, 0, len(c.trustedRoots.RawCertificates()))
	for _, cert := range c.trustedRoots.RawCertificates() {
		rawCerts = append(rawCerts, cert.Raw)
	}

	jsonMap := make(map[string]interface{})
	jsonMap[jsonMapKeyCertificates] = rawCerts
	enc := json.NewEncoder(w)
	err := enc.Encode(jsonMap)
	if err != nil {
		glog.Warningf("%s: get_roots failed: %v", c.logPrefix, err)
		return http.StatusInternalServerError, fmt.Errorf("get-roots failed with: %v", err)
	}

	return http.StatusOK, nil
}

// See RFC 6962 Section 4.8. This is mostly used for debug purposes rather than by normal
// CT clients.
func getEntryAndProof(ctx context.Context, c LogContext, w http.ResponseWriter, r *http.Request) (int, error) {
	// Ensure both numeric params are present and look reasonable.
	leafIndex, treeSize, err := parseGetEntryAndProofParams(r)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("failed to parse get-entry-and-proof params: %v", err)
	}

	req := trillian.GetEntryAndProofRequest{LogId: c.logID, LeafIndex: leafIndex, TreeSize: treeSize}
	rsp, err := c.rpcClient.GetEntryAndProof(ctx, &req)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("backend GetEntryAndProof request failed: %v", err)
	}
	if !rpcStatusOK(rsp.GetStatus()) {
		return http.StatusInternalServerError, fmt.Errorf("backend GetEntryAndProof request failed, status=%v", rsp.GetStatus())
	}

	// Apply some checks that we got reasonable data from the backend
	if rsp.Proof == nil || rsp.Leaf == nil || len(rsp.Proof.ProofNode) == 0 || len(rsp.Leaf.LeafValue) == 0 {
		return http.StatusInternalServerError, fmt.Errorf("got RPC bad response, possible extra info: %v", rsp)
	}

	// Build and marshal the response to the client
	jsonRsp := ct.GetEntryAndProofResponse{
		LeafInput: rsp.Leaf.LeafValue,
		ExtraData: rsp.Leaf.ExtraData,
		AuditPath: auditPathFromProto(rsp.Proof.ProofNode),
	}

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	jsonData, err := json.Marshal(&jsonRsp)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-entry-and-proof resp: %v because: %v", jsonRsp, err)
	}

	_, err = w.Write(jsonData)
	if err != nil {

		// Probably too late for this as headers might have been written but we don't know for sure
		return http.StatusInternalServerError, fmt.Errorf("failed to write get-entry-and-proof resp: %v because: %v", jsonRsp, err)
	}

	return http.StatusOK, nil
}

// RegisterHandlers registers a HandleFunc for all of the RFC6962 defined methods.
// TODO(Martin2112): This registers on default ServeMux, might need more flexibility?
func (c LogContext) RegisterHandlers() {
	// Bind the LogContext instance to give an appHandler instance for each entrypoint.
	http.Handle(ct.AddChainPath, appHandler{context: c, handler: addChain, name: "AddChain", method: http.MethodPost})
	http.Handle(ct.AddPreChainPath, appHandler{context: c, handler: addPreChain, name: "AddPreChain", method: http.MethodPost})
	http.Handle(ct.GetSTHPath, appHandler{context: c, handler: getSTH, name: "GetSTH", method: http.MethodGet})
	http.Handle(ct.GetSTHConsistencyPath, appHandler{context: c, handler: getSTHConsistency, name: "GetSTHConsistency", method: http.MethodGet})
	http.Handle(ct.GetProofByHashPath, appHandler{context: c, handler: getProofByHash, name: "GetProofByHash", method: http.MethodGet})
	http.Handle(ct.GetEntriesPath, appHandler{context: c, handler: getEntries, name: "GetEntries", method: http.MethodGet})
	http.Handle(ct.GetRootsPath, appHandler{context: c, handler: getRoots, name: "GetRoots", method: http.MethodGet})
	http.Handle(ct.GetEntryAndProofPath, appHandler{context: c, handler: getEntryAndProof, name: "GetEntryAndProof", method: http.MethodGet})
}

// Generates a custom error page to give more information on why something didn't work
// TODO(Martin2112): Not sure if we want to expose any detail or not
func sendHTTPError(w http.ResponseWriter, statusCode int, err error) {
	http.Error(w, fmt.Sprintf("%s\n%v", http.StatusText(statusCode), err), statusCode)
}

// getRPCDeadlineTime calculates the future time an RPC should expire based on our config
func getRPCDeadlineTime(c LogContext) time.Time {
	return c.timeSource.Now().Add(c.rpcDeadline)
}

func rpcStatusOK(status *trillian.TrillianApiStatus) bool {
	return status != nil && status.StatusCode == trillian.TrillianApiStatusCode_OK
}

// verifyAddChain is used by add-chain and add-pre-chain. It does the checks that the supplied
// cert is of the correct type and chains to a trusted root.
// TODO(Martin2112): This may not implement all the RFC requirements. Check what is provided
// by fixchain (called by this code) plus the ones here to make sure that it is compliant.
func verifyAddChain(c LogContext, req ct.AddChainRequest, w http.ResponseWriter, expectingPrecert bool) ([]*x509.Certificate, error) {
	// We already checked that the chain is not empty so can move on to verification
	validPath, err := ValidateChain(req.Chain, *c.trustedRoots)
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
			glog.Warningf("%s: Cert (or precert with invalid CT ext) submitted as precert chain: %v", c.logPrefix, req)
		} else {
			glog.Warningf("%s: Precert (or cert with invalid CT ext) submitted as cert chain: %v", c.logPrefix, req)
		}
		return nil, fmt.Errorf("cert / precert mismatch: %v", expectingPrecert)
	}

	return validPath, nil
}

// buildLogLeafForAddChain is also used by add-pre-chain and does the hashing to build a
// LogLeaf that will be sent to the backend
func buildLogLeafForAddChain(c LogContext, merkleLeaf ct.MerkleTreeLeaf, chain []*x509.Certificate) (trillian.LogLeaf, error) {
	leafData, err := tls.Marshal(merkleLeaf)
	if err != nil {
		glog.Warningf("%s: Failed to serialize Merkle leaf: %v", c.logPrefix, err)
		return trillian.LogLeaf{}, err
	}

	entry := NewLogEntry(merkleLeaf, chain)
	entryData, err := tls.Marshal(*entry)
	if err != nil {
		glog.Warningf("%s: Failed to serialize log entry: %v", c.logPrefix, err)
		return trillian.LogLeaf{}, err
	}

	// leafHash is a crosscheck on the data we're sending in the leaf buffer. The backend
	// does the tree hashing.
	leafHash := sha256.Sum256(leafData)

	return trillian.LogLeaf{
		LeafValueHash: leafHash[:],
		LeafValue:     leafData,
		ExtraData:     entryData,
	}, nil
}

// marshalAndWriteAddChainResponse is used by add-chain and add-pre-chain to create and write
// the JSON response to the client
func marshalAndWriteAddChainResponse(sct ct.SignedCertificateTimestamp, km crypto.KeyManager, w http.ResponseWriter) error {
	logID, err := GetCTLogID(km)
	if err != nil {
		return fmt.Errorf("failed to marshal logID: %v", err)
	}
	sig, err := tls.Marshal(sct.Signature)
	if err != nil {
		return fmt.Errorf("failed to marshal signature: %v", err)
	}

	rsp := ct.AddChainResponse{
		SCTVersion: ct.Version(sct.SCTVersion),
		Timestamp:  sct.Timestamp,
		ID:         logID[:],
		Extensions: "",
		Signature:  sig,
	}

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	jsonData, err := json.Marshal(&rsp)
	if err != nil {
		return fmt.Errorf("failed to marshal add-chain resp: %v because: %v", rsp, err)
	}

	_, err = w.Write(jsonData)
	if err != nil {
		return fmt.Errorf("failed to write add-chain resp: %v", rsp)
	}

	return nil
}

func parseGetEntriesRange(r *http.Request, maxRange int64) (int64, int64, error) {
	start, err := strconv.ParseInt(r.FormValue(getEntriesParamStart), 10, 64)
	if err != nil {
		return 0, 0, err
	}

	end, err := strconv.ParseInt(r.FormValue(getEntriesParamEnd), 10, 64)
	if err != nil {
		return 0, 0, err
	}

	if start < 0 || end < 0 {
		return 0, 0, fmt.Errorf("start (%d) and end (%d) parameters must be >= 0", start, end)
	}
	if start > end {
		return 0, 0, fmt.Errorf("start (%d) and end (%d) is not a valid range", start, end)
	}

	count := end - start + 1
	if count > maxRange {
		return 0, 0, fmt.Errorf("requesting %d entries but we only allow up to %d", count, maxRange)
	}

	return start, end, nil
}

func parseGetEntryAndProofParams(r *http.Request) (int64, int64, error) {
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

func parseGetSTHConsistencyRange(r *http.Request) (int64, int64, error) {
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
func isResponseContiguousRange(rsp *trillian.GetLeavesByIndexResponse, start, end int64) error {
	for li, l := range rsp.Leaves {
		if li > 0 && rsp.Leaves[li].LeafIndex-rsp.Leaves[li-1].LeafIndex != 1 {
			return fmt.Errorf("backend returned non contiguous leaves: %v %v", rsp.Leaves[li-1], rsp.Leaves[li])
		}

		if l.LeafIndex < start || l.LeafIndex > end {
			return fmt.Errorf("backend returned leaf:%d outside requested range:%d, %d", l.LeafIndex, start, end)
		}
	}

	return nil
}

// marshalGetEntriesResponse does the conversion from the backend response to the one we need for
// an RFC compliant JSON response to the client.
func marshalGetEntriesResponse(c LogContext, rsp *trillian.GetLeavesByIndexResponse) (ct.GetEntriesResponse, error) {
	jsonRsp := ct.GetEntriesResponse{}

	for _, leaf := range rsp.Leaves {
		// We're only deserializing it to ensure it's valid, don't need the result. We still
		// return the data if it fails to deserialize as otherwise the root hash could not
		// be verified. However this indicates a potentially serious failure in log operation
		// or data storage that should be investigated.
		var treeLeaf ct.MerkleTreeLeaf
		if rest, err := tls.Unmarshal(leaf.LeafValue, &treeLeaf); err != nil {
			// TODO(Martin2112): Hook this up to monitoring when implemented
			glog.Warningf("%s: Failed to deserialize Merkle leaf from backend: %d", c.logPrefix, leaf.LeafIndex)
		} else if len(rest) > 0 {
			glog.Warningf("%s: Trailing data after Merkle leaf from backend: %d", c.logPrefix, leaf.LeafIndex)
		}

		jsonRsp.Entries = append(jsonRsp.Entries, ct.LeafEntry{
			LeafInput: leaf.LeafValue,
			ExtraData: leaf.ExtraData,
		})
	}

	return jsonRsp, nil
}

// checkAuditPath does a quick scan of the proof we got from the backend for consistency.
// All the hashes should be non zero length.
// TODO(Martin2112): should maybe check they are all the same length and all the expected
// length of the hashes used in the RFC.
func checkAuditPath(path []*trillian.Node) bool {
	for _, node := range path {
		if len(node.NodeHash) == 0 {
			return false
		}
	}
	return true
}

// auditPathFromProto converts the path from proof proto to a format we can return in the JSON
// response
func auditPathFromProto(path []*trillian.Node) [][]byte {
	result := make([][]byte, 0, len(path))
	for _, node := range path {
		result = append(result, node.NodeHash)
	}
	return result
}
