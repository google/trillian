// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ct

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/tls"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/util"
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
	// The name of the get-entry-and-proof tree size parameter
	getEntryAndProofParamTreeSize = "tree_size"
)

// EntrypointName identifies a CT entrypoint as defined in section 4 of RFC 6962.
type EntrypointName string

// Constants for entrypoint names, as exposed in statistics/logging.
const (
	AddChainName          = EntrypointName("AddChain")
	AddPreChainName       = EntrypointName("AddPreChain")
	GetSTHName            = EntrypointName("GetSTH")
	GetSTHConsistencyName = EntrypointName("GetSTHConsistency")
	GetProofByHashName    = EntrypointName("GetProofByHash")
	GetEntriesName        = EntrypointName("GetEntries")
	GetRootsName          = EntrypointName("GetRoots")
	GetEntryAndProofName  = EntrypointName("GetEntryAndProof")
)

// Entrypoints is a list of entrypoint names as exposed in statistics/logging.
var Entrypoints = []EntrypointName{AddChainName, AddPreChainName, GetSTHName, GetSTHConsistencyName, GetProofByHashName, GetEntriesName, GetRootsName, GetEntryAndProofName}

// PathHandlers maps from a path to the relevant AppHandler instance.
type PathHandlers map[string]AppHandler

// AppHandler holds a LogContext and a handler function that uses it, and is
// an implementation of the http.Handler interface.
type AppHandler struct {
	Context LogContext
	Handler func(context.Context, LogContext, http.ResponseWriter, *http.Request) (int, error)
	Name    EntrypointName
	Method  string // http.MethodGet or http.MethodPost
}

// ServeHTTP for an AppHandler invokes the underlying handler function but
// does additional common error and stats processing.
func (a AppHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.Context.exp.vars.Add("http-all-reqs", 1)
	a.Context.exp.reqs.Add(string(a.Name), 1)
	glog.V(2).Infof("%s: request %v %q => %s", a.Context.LogPrefix, r.Method, r.URL, a.Name)
	if r.Method != a.Method {
		glog.Warningf("%s: %s wrong HTTP method: %v", a.Context.LogPrefix, a.Name, r.Method)
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
	ctx, cancel := context.WithDeadline(r.Context(), getRPCDeadlineTime(a.Context))
	defer cancel()

	status, err := a.Handler(ctx, a.Context, w, r)
	glog.V(2).Infof("%s: %s <= status=%d", a.Context.LogPrefix, a.Name, status)
	a.Context.exp.allRsps.Add(strconv.Itoa(status), 1)
	e := a.Context.exp.rsps.Get(string(a.Name))
	if e, ok := e.(*expvar.Map); ok {
		e.Add(strconv.Itoa(status), 1)
	}
	if err != nil {
		glog.Warningf("%s: %s handler error: %v", a.Context.LogPrefix, a.Name, err)
		sendHTTPError(w, status, err)
		return
	}

	// Additional check, for consistency the handler must return an error for non-200 status
	if status != http.StatusOK {
		glog.Warningf("%s: %s handler non 200 without error: %d %v", a.Context.LogPrefix, a.Name, status, err)
		sendHTTPError(w, http.StatusInternalServerError, fmt.Errorf("http handler misbehaved, status: %d", status))
		return
	}
}

// LogContext holds information for a specific log instance.
type LogContext struct {
	// LogPrefix is a pre-formatted string identifying the log for diagnostics
	LogPrefix string
	// TimeSource is a util.TimeSource that can be injected for testing
	TimeSource util.TimeSource

	// logID is the tree ID that identifies this log in node storage
	logID int64
	// urlPrefix is the prefix for URLs for this log
	urlPrefix string
	// trustedRoots is a pool of certificates that defines the roots the CT log will accept
	trustedRoots *PEMCertPool
	// rpcClient is the client used to communicate with the trillian backend
	rpcClient trillian.TrillianLogClient
	// signer signs objects
	signer *crypto.Signer
	// rpcDeadline is the deadline that will be set on all backend RPC requests
	rpcDeadline time.Duration
	// Various per-log statistics
	exp struct {
		vars             *expvar.Map // varname => expvar.Var, includes all below
		lastSCTTimestamp *expvar.Int
		lastSTHTimestamp *expvar.Int
		lastSTHTreeSize  *expvar.Int
		// Statistics for HTTP requests/responses
		reqs    *expvar.Map // entrypoint => expvar.Int  (as "http-reqs")
		allRsps *expvar.Map // http.rc => expvar.Int  (as "http-all-rsps")
		rsps    *expvar.Map // entrypoint => expvar.Map[http.rc => expvar.Int]  (as "http-rsps")
	}
}

// NewLogContext creates a new instance of LogContext.
func NewLogContext(logID int64, prefix string, trustedRoots *PEMCertPool, rpcClient trillian.TrillianLogClient, signer *crypto.Signer, rpcDeadline time.Duration, timeSource util.TimeSource) *LogContext {
	ctx := &LogContext{
		logID:        logID,
		urlPrefix:    prefix,
		LogPrefix:    fmt.Sprintf("%s{%d}", prefix, logID),
		trustedRoots: trustedRoots,
		rpcClient:    rpcClient,
		signer:       signer,
		rpcDeadline:  rpcDeadline,
		TimeSource:   timeSource,
	}

	// Initialize all the exported variables.
	ctx.exp.vars = new(expvar.Map).Init()

	e := new(expvar.Int)
	e.Set(logID)
	ctx.exp.vars.Set("log-id", e)
	ctx.exp.lastSCTTimestamp = new(expvar.Int)
	ctx.exp.vars.Set("last-sct-timestamp", ctx.exp.lastSCTTimestamp)
	ctx.exp.lastSTHTimestamp = new(expvar.Int)
	ctx.exp.vars.Set("last-sth-timestamp", ctx.exp.lastSTHTimestamp)
	ctx.exp.lastSTHTreeSize = new(expvar.Int)
	ctx.exp.vars.Set("last-sth-treesize", ctx.exp.lastSTHTreeSize)

	// TODO(drysdale): investigate whether there's a generic wrapper to do this
	ctx.exp.reqs = new(expvar.Map).Init()
	ctx.exp.vars.Set("http-reqs", ctx.exp.reqs)
	ctx.exp.allRsps = new(expvar.Map).Init()
	ctx.exp.vars.Set("http-all-rsps", ctx.exp.allRsps)
	ctx.exp.rsps = new(expvar.Map).Init()
	for _, ep := range Entrypoints {
		ctx.exp.rsps.Set(string(ep), new(expvar.Map).Init())
	}
	ctx.exp.vars.Set("http-rsps", ctx.exp.rsps)

	return ctx
}

// Handlers returns a map from URL paths (with the given prefix) and AppHandler instances
// to handle those entrypoints.
func (c LogContext) Handlers(prefix string) PathHandlers {
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimRight(prefix, "/")

	// Bind the LogContext instance to give an appHandler instance for each entrypoint.
	return PathHandlers{
		prefix + ct.AddChainPath:          AppHandler{Context: c, Handler: addChain, Name: AddChainName, Method: http.MethodPost},
		prefix + ct.AddPreChainPath:       AppHandler{Context: c, Handler: addPreChain, Name: AddPreChainName, Method: http.MethodPost},
		prefix + ct.GetSTHPath:            AppHandler{Context: c, Handler: getSTH, Name: GetSTHName, Method: http.MethodGet},
		prefix + ct.GetSTHConsistencyPath: AppHandler{Context: c, Handler: getSTHConsistency, Name: GetSTHConsistencyName, Method: http.MethodGet},
		prefix + ct.GetProofByHashPath:    AppHandler{Context: c, Handler: getProofByHash, Name: GetProofByHashName, Method: http.MethodGet},
		prefix + ct.GetEntriesPath:        AppHandler{Context: c, Handler: getEntries, Name: GetEntriesName, Method: http.MethodGet},
		prefix + ct.GetRootsPath:          AppHandler{Context: c, Handler: getRoots, Name: GetRootsName, Method: http.MethodGet},
		prefix + ct.GetEntryAndProofPath:  AppHandler{Context: c, Handler: getEntryAndProof, Name: GetEntryAndProofName, Method: http.MethodGet},
	}
}

func parseBodyAsJSONChain(c LogContext, r *http.Request) (ct.AddChainRequest, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		glog.V(1).Infof("%s: Failed to read request body: %v", c.LogPrefix, err)
		return ct.AddChainRequest{}, err
	}

	var req ct.AddChainRequest
	if err := json.Unmarshal(body, &req); err != nil {
		glog.V(1).Infof("%s: Failed to parse request body: %v", c.LogPrefix, err)
		return ct.AddChainRequest{}, err
	}

	// The cert chain is not allowed to be empty. We'll defer other validation for later
	if len(req.Chain) == 0 {
		glog.V(1).Infof("%s: Request chain is empty: %s", c.LogPrefix, body)
		return ct.AddChainRequest{}, errors.New("cert chain was empty")
	}

	return req, nil
}

// addChainInternal is called by add-chain and add-pre-chain as the logic involved in
// processing these requests is almost identical
// TODO(Martin2112): Doesn't properly handle duplicate submissions yet but the backend
// needs this to be implemented before we can do it here
func addChainInternal(ctx context.Context, c LogContext, w http.ResponseWriter, r *http.Request, isPrecert bool) (int, error) {
	var signerFn func(*crypto.Signer, *x509.Certificate, *x509.Certificate, time.Time) (*ct.MerkleTreeLeaf, *ct.SignedCertificateTimestamp, error)
	var method EntrypointName
	if isPrecert {
		method = AddPreChainName
		signerFn = signV1SCTForPrecertificate
	} else {
		method = AddChainName
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
	var issuer *x509.Certificate
	if len(chain) > 1 {
		issuer = chain[1]
	}
	merkleLeaf, sct, err := signerFn(c.signer, chain[0], issuer, c.TimeSource.Now())
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to build SCT and Merkle leaf: %v %v", sct, err)
	}

	// Send the Merkle tree leaf on to the Log server.
	leaf, err := buildLogLeafForAddChain(c, *merkleLeaf, chain)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to build LogLeaf: %v", err)
	}
	req := trillian.QueueLeavesRequest{LogId: c.logID, Leaves: []*trillian.LogLeaf{&leaf}}

	glog.V(2).Infof("%s: %s => grpc.QueueLeaves", c.LogPrefix, method)
	_, err = c.rpcClient.QueueLeaves(ctx, &req)
	glog.V(2).Infof("%s: %s <= grpc.QueueLeaves err=%v", c.LogPrefix, method, err)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("backend QueueLeaves request failed: %v", err)
	}

	// As the Log server has successfully queued up the Merkle tree leaf, we can
	// respond with an SCT.
	err = marshalAndWriteAddChainResponse(sct, c.signer, w)
	if err != nil {
		// reason is logged and http status is already set
		// TODO(Martin2112): Record failure for monitoring when it's implemented
		return http.StatusInternalServerError, fmt.Errorf("failed to write response: %v", err)
	}
	glog.V(3).Infof("%s: %s <= SCT", c.LogPrefix, method)
	c.exp.lastSCTTimestamp.Set(int64(sct.Timestamp))

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
	glog.V(2).Infof("%s: GetSTH => grpc.GetLatestSignedLogRoot %+v", c.LogPrefix, req)
	rsp, err := c.rpcClient.GetLatestSignedLogRoot(ctx, &req)
	glog.V(2).Infof("%s: GetSTH <= grpc.GetLatestSignedLogRoot err=%v", c.LogPrefix, err)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("backend GetLatestSignedLogRoot request failed: %v", err)
	}

	// Check over the response.
	slr := rsp.GetSignedLogRoot()
	if slr == nil {
		return http.StatusInternalServerError, fmt.Errorf("no log root returned")
	}
	glog.V(3).Infof("%s: GetSTH <= slr=%+v", c.LogPrefix, slr)
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
	err = signV1TreeHead(c.signer, &sth)
	if err != nil || len(sth.TreeHeadSignature.Signature) == 0 {
		return http.StatusInternalServerError, fmt.Errorf("failed to sign tree head: %v", err)
	}

	// Now build the final result object that will be marshaled to JSON
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
	c.exp.lastSTHTimestamp.Set(int64(sth.Timestamp))
	c.exp.lastSTHTreeSize.Set(int64(sth.TreeSize))

	return http.StatusOK, nil
}

func getSTHConsistency(ctx context.Context, c LogContext, w http.ResponseWriter, r *http.Request) (int, error) {
	first, second, err := parseGetSTHConsistencyRange(r)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("failed to parse consistency range: %v", err)
	}
	req := trillian.GetConsistencyProofRequest{LogId: c.logID, FirstTreeSize: first, SecondTreeSize: second}

	glog.V(2).Infof("%s: GetSTHConsistency(%d, %d) => grpc.GetConsistencyProof %+v", c.LogPrefix, first, second, req)
	rsp, err := c.rpcClient.GetConsistencyProof(ctx, &req)
	glog.V(2).Infof("%s: GetSTHConsistency <= grpc.GetConsistencyProof err=%v", c.LogPrefix, err)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("backend GetConsistencyProof request failed: %v", err)
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
	escapedHash := r.FormValue(getProofParamHash)
	if len(escapedHash) == 0 {
		return http.StatusBadRequest, errors.New("get-proof-by-hash: missing / empty hash param for get-proof-by-hash")
	}
	hash, err := url.QueryUnescape(escapedHash)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("get-proof-by-hash: invalid url-encoded hash: %v", err)
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

	// Additional sanity checks, none of the hashes in the returned path should be empty
	if len(rsp.Proof) == 0 {
		return http.StatusInternalServerError, fmt.Errorf("get-proof-by-hash: backend returned empty proof")
	}
	if !checkAuditPath(rsp.Proof[0].ProofNode) {
		return http.StatusInternalServerError, fmt.Errorf("get-proof-by-hash: backend returned invalid proof: %v", rsp.Proof[0])
	}

	// All checks complete, marshal and return the response
	proofRsp := ct.GetProofByHashResponse{LeafIndex: rsp.Proof[0].LeafIndex, AuditPath: auditPathFromProto(rsp.Proof[0].ProofNode)}

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	jsonData, err := json.Marshal(&proofRsp)
	if err != nil {
		glog.Warningf("%s: Failed to marshal get-proof-by-hash resp: %v", c.LogPrefix, proofRsp)
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

	// Trillian doesn't guarantee the returned leaves are in order (they don't need to be
	// because each leaf comes with an index).  CT doesn't expose an index field and so
	// needs to return leaves in order.  Therefore, sort the results (and check for missing
	// or duplicate indices along the way).
	if err := sortLeafRange(rsp, start, end); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("backend get-entries range invalid: %v", err)
	}

	// Now we've checked the RPC response and it seems to be valid we need
	// to serialize the leaves in JSON format for the HTTP response. Doing a
	// round trip via the leaf deserializer gives us another chance to
	// prevent bad / corrupt data from reaching the client.
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
		glog.Warningf("%s: get_roots failed: %v", c.LogPrefix, err)
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

// Generates a custom error page to give more information on why something didn't work
// TODO(Martin2112): Not sure if we want to expose any detail or not
func sendHTTPError(w http.ResponseWriter, statusCode int, err error) {
	http.Error(w, fmt.Sprintf("%s\n%v", http.StatusText(statusCode), err), statusCode)
}

// getRPCDeadlineTime calculates the future time an RPC should expire based on our config
func getRPCDeadlineTime(c LogContext) time.Time {
	return c.TimeSource.Now().Add(c.rpcDeadline)
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
			glog.Warningf("%s: Cert (or precert with invalid CT ext) submitted as precert chain: %v", c.LogPrefix, req)
		} else {
			glog.Warningf("%s: Precert (or cert with invalid CT ext) submitted as cert chain: %v", c.LogPrefix, req)
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
		glog.Warningf("%s: Failed to serialize Merkle leaf: %v", c.LogPrefix, err)
		return trillian.LogLeaf{}, err
	}

	isPrecert, err := IsPrecertificate(chain[0])
	if err != nil {
		glog.Warningf("%s: Failed to determine if cert or pre-cert: %v", c.LogPrefix, err)
		return trillian.LogLeaf{}, err
	}

	extraData, err := extraDataForChain(chain, isPrecert)
	if err != nil {
		glog.Warningf("%s: Failed to serialize chain for ExtraData: %v", c.LogPrefix, err)
		return trillian.LogLeaf{}, err
	}

	// leafIDHash allows Trillian to detect duplicate entries, so this should be
	// a hash over the cert data.
	leafIDHash := sha256.Sum256(chain[0].Raw)

	return trillian.LogLeaf{
		LeafIdentityHash: leafIDHash[:],
		LeafValue:        leafData,
		ExtraData:        extraData,
	}, nil
}

// extraDataForChain creates the extra data associated with a log entry as described in
// RFC6962 section 4.6.
func extraDataForChain(chain []*x509.Certificate, isPrecert bool) ([]byte, error) {
	var extraData []byte
	var err error
	if isPrecert {
		// For a pre-certificate, the extra data is a TLS-encoded PrecertChainEntry.
		extra := ct.PrecertChainEntry{
			PreCertificate:   ct.ASN1Cert{Data: chain[0].Raw},
			CertificateChain: make([]ct.ASN1Cert, len(chain)-1),
		}
		for i := 1; i < len(chain); i++ {
			extra.CertificateChain[i-1] = ct.ASN1Cert{Data: chain[i].Raw}
		}
		extraData, err = tls.Marshal(extra)
		if err != nil {
			return nil, err
		}
	} else {
		// For a certificate, the extra data is a TLS-encoded:
		//   ASN.1Cert certificate_chain<0..2^24-1>;
		// containing the chain after the leaf.
		extra := ct.CertificateChain{
			Entries: make([]ct.ASN1Cert, len(chain)-1),
		}
		for i := 1; i < len(chain); i++ {
			extra.Entries[i-1] = ct.ASN1Cert{Data: chain[i].Raw}
		}
		extraData, err = tls.Marshal(extra)
		if err != nil {
			return nil, err
		}
	}
	return extraData, nil
}

// marshalAndWriteAddChainResponse is used by add-chain and add-pre-chain to create and write
// the JSON response to the client
func marshalAndWriteAddChainResponse(sct *ct.SignedCertificateTimestamp, signer *crypto.Signer, w http.ResponseWriter) error {
	logID, err := GetCTLogID(signer.Public())
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

type byLeafIndex []*trillian.LogLeaf

func (ll byLeafIndex) Len() int {
	return len(ll)
}
func (ll byLeafIndex) Swap(i, j int) {
	ll[i], ll[j] = ll[j], ll[i]
}
func (ll byLeafIndex) Less(i, j int) bool {
	return ll[i].LeafIndex < ll[j].LeafIndex
}

// sortLeafRange re-orders the leaves in rsp to be in ascending order by LeafIndex.  It also
// checks that the resulting range of leaves in rsp is valid, starting at start and finishing
// at end (or before) without duplicates.
func sortLeafRange(rsp *trillian.GetLeavesByIndexResponse, start, end int64) error {
	if got := int64(len(rsp.Leaves)); got > (end + 1 - start) {
		return fmt.Errorf("backend returned too many leaves: %d v [%d,%d]", got, start, end)
	}
	sort.Sort(byLeafIndex(rsp.Leaves))
	for i, leaf := range rsp.Leaves {
		if leaf.LeafIndex != (start + int64(i)) {
			return fmt.Errorf("backend returned unexpected leaf index: rsp.Leaves[%d].LeafIndex=%d for range [%d,%d]", i, leaf.LeafIndex, start, end)
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
			glog.Warningf("%s: Failed to deserialize Merkle leaf from backend: %d", c.LogPrefix, leaf.LeafIndex)
		} else if len(rest) > 0 {
			glog.Warningf("%s: Trailing data after Merkle leaf from backend: %d", c.LogPrefix, leaf.LeafIndex)
		}

		extraData := leaf.ExtraData
		if len(extraData) == 0 {
			glog.Errorf("%s: Missing ExtraData for leaf %d", c.LogPrefix, leaf.LeafIndex)
		}
		jsonRsp.Entries = append(jsonRsp.Entries, ct.LeafEntry{
			LeafInput: leaf.LeafValue,
			ExtraData: extraData,
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
