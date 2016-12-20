//+build integration

package integration

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/client"
	"github.com/google/certificate-transparency/go/jsonclient"
	"github.com/google/certificate-transparency/go/merkletree"
	"github.com/google/certificate-transparency/go/tls"
	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/certificate-transparency/go/x509/pkix"
	"github.com/google/trillian/crypto"
	ctfe "github.com/google/trillian/examples/ct"
	"github.com/google/trillian/testonly"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

var httpServerFlag = flag.String("ct_http_server", "localhost:8092", "Server address:port")
var testDir = flag.String("testdata", "testdata", "Name of directory with test data")
var seed = flag.Int64("seed", -1, "Seed for random number generation")
var logConfigFlag = flag.String("log_config", "", "File holding log config in JSON")

// TODO(drysdale): convert to use trillian/merkle to avoid the dependency on the
// CT code (which in turn requires C++ code).

var verifier = merkletree.NewMerkleVerifier(func(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
})

func TestCTIntegration(t *testing.T) {
	flag.Parse()
	if *seed == -1 {
		*seed = time.Now().UTC().UnixNano() & 0xFFFFFFFF
	}
	fmt.Printf("Today's test has been brought to you by the letters C and T and the number %#x\n", *seed)
	rand.Seed(*seed)

	cfg, err := ctfe.LogConfigFromFile(*logConfigFlag)
	if err != nil {
		t.Fatalf("Failed to read log config: %v", err)
	}

	type result struct {
		prefix string
		err    error
	}
	results := make(chan result, len(cfg))
	var wg sync.WaitGroup
	for _, c := range cfg {
		wg.Add(1)
		go func(c ctfe.LogConfig) {
			defer wg.Done()
			err := testCTIntegrationForLog(c)
			results <- result{prefix: c.Prefix, err: err}
		}(c)
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e.err != nil {
			t.Errorf("%s: %v", e.prefix, e.err)
		}
	}
}

// Run tests against the log with configuration cfg.
func testCTIntegrationForLog(cfg ctfe.LogConfig) error {
	opts := jsonclient.Options{}
	if cfg.PubKeyPEMFile != "" {
		pubkey, err := ioutil.ReadFile(cfg.PubKeyPEMFile)
		if err != nil {
			return fmt.Errorf("failed to get public key contents: %v", err)
		}
		opts.PublicKey = string(pubkey)
	}
	logURI := "http://" + (*httpServerFlag) + "/" + cfg.Prefix
	logClient, err := client.New(logURI, nil, opts)
	if err != nil {
		return fmt.Errorf("failed to create LogClient instance: %v", err)
	}
	ctx := context.Background()
	stats := newWantStats(cfg.LogID)
	if err := stats.check(cfg); err != nil {
		return fmt.Errorf("unexpected stats check: %v", err)
	}

	// Stage 0: get accepted roots, which should just be the fake CA.
	roots, err := logClient.GetAcceptedRoots(ctx)
	stats.done("GetRoots", 200)
	if err != nil {
		return fmt.Errorf("got GetAcceptedRoots()=(nil,%v); want (_,nil)", err)
	}
	if len(roots) != 1 {
		return fmt.Errorf("len(GetAcceptedRoots())=%d; want 1", len(roots))
	}

	// Stage 1: get the STH, which should be empty.
	sth0, err := logClient.GetSTH(ctx)
	stats.done("GetSTH", 200)
	if err != nil {
		return fmt.Errorf("got GetSTH()=(nil,%v); want (_,nil)", err)
	}
	if sth0.Version != 0 {
		return fmt.Errorf("sth.Version=%v; want V1(0)", sth0.Version)
	}
	if sth0.TreeSize != 0 {
		return fmt.Errorf("sth.TreeSize=%d; want 0", sth0.TreeSize)
	}
	fmt.Printf("%s: Got STH(time=%q, size=%d): roothash=%x\n", cfg.Prefix, ctTime(sth0.Timestamp), sth0.TreeSize, sth0.SHA256RootHash)

	// Stage 2: add a single cert (the intermediate CA), get an SCT.
	var scts [21]*ct.SignedCertificateTimestamp // 0=int-ca, 1-20=leaves
	var chain [21][]ct.ASN1Cert
	chain[0], err = getChain("int-ca.cert")
	if err != nil {
		return fmt.Errorf("failed to load certificate: %v", err)
	}
	scts[0], err = logClient.AddChain(ctx, chain[0])
	stats.done("AddChain", 200)
	if err != nil {
		return fmt.Errorf("got AddChain(int-ca.cert)=(nil,%v); want (_,nil)", err)
	}
	// Display the SCT
	fmt.Printf("%s: Uploaded int-ca.cert to %v log, got SCT(time=%q)\n", cfg.Prefix, scts[0].SCTVersion, ctTime(scts[0].Timestamp))

	// Keep getting the STH until tree size becomes 1.
	sth1, err := awaitTreeSize(ctx, logClient, 1, true, &stats)
	if err != nil {
		return fmt.Errorf("awaitTreeSize(1)=(nil,%v); want (_,nil)", err)
	}
	fmt.Printf("%s: Got STH(time=%q, size=%d): roothash=%x\n", cfg.Prefix, ctTime(sth1.Timestamp), sth1.TreeSize, sth1.SHA256RootHash)
	if err := stats.check(cfg); err != nil {
		return fmt.Errorf("unexpected stats check: %v", err)
	}

	// Stage 3: add a second cert, wait for tree size = 2
	chain[1], err = getChain("leaf01.chain")
	if err != nil {
		return fmt.Errorf("failed to load certificate: %v", err)
	}
	scts[1], err = logClient.AddChain(ctx, chain[1])
	stats.done("AddChain", 200)
	if err != nil {
		return fmt.Errorf("got AddChain(leaf01)=(nil,%v); want (_,nil)", err)
	}
	fmt.Printf("%s: Uploaded cert01.chain to %v log, got SCT(time=%q)\n", cfg.Prefix, scts[1].SCTVersion, ctTime(scts[1].Timestamp))
	sth2, err := awaitTreeSize(ctx, logClient, 2, true, &stats)
	if err != nil {
		return fmt.Errorf("failed to get STH for size=1: %v", err)
	}
	fmt.Printf("%s: Got STH(time=%q, size=%d): roothash=%x\n", cfg.Prefix, ctTime(sth2.Timestamp), sth2.TreeSize, sth2.SHA256RootHash)

	// Stage 4: get a consistency proof from size 1-> size 2.
	proof12, err := logClient.GetSTHConsistency(ctx, 1, 2)
	stats.done("GetSTHConsistency", 200)
	if err != nil {
		return fmt.Errorf("got GetSTHConsistency(1, 2)=(nil,%v); want (_,nil)", err)
	}
	//                 sth2
	//                 / \
	//  sth1   =>      a b
	//    |            | |
	//   d0           d0 d1
	// So consistency proof is [b] and we should have:
	//   sth2 == SHA256(0x01 | sth1 | b)
	if len(proof12) != 1 {
		return fmt.Errorf("len(proof12)=%d; want 1", len(proof12))
	}
	if err := checkCTConsistencyProof(sth1, sth2, proof12); err != nil {
		return fmt.Errorf("checkCTConsistencyProof(sth1,sth2,proof12)=%v; want nil", err)
	}
	if err := stats.check(cfg); err != nil {
		return fmt.Errorf("unexpected stats check: %v", err)
	}

	// Stage 5: add certificates 2, 3, 4, 5,...N, for some random N in [4,20]
	atLeast := 4
	count := atLeast + rand.Intn(20-atLeast)
	for i := 2; i <= count; i++ {
		filename := fmt.Sprintf("leaf%02d.chain", i)
		chain[i], err = getChain(filename)
		if err != nil {
			return fmt.Errorf("failed to load certificate: %v", err)
		}
		scts[i], err = logClient.AddChain(ctx, chain[i])
		stats.done("AddChain", 200)
		if err != nil {
			return fmt.Errorf("got AddChain(leaf%02d)=(nil,%v); want (_,nil)", i, err)
		}
	}
	fmt.Printf("%s: Uploaded leaf02-leaf%02d to log, got SCTs\n", cfg.Prefix, count)
	if err := stats.check(cfg); err != nil {
		return fmt.Errorf("unexpected stats check: %v", err)
	}

	// Stage 6: keep getting the STH until tree size becomes 1 + N (allows for int-ca.cert).
	treeSize := 1 + count
	sthN, err := awaitTreeSize(ctx, logClient, uint64(treeSize), true, &stats)
	if err != nil {
		return fmt.Errorf("awaitTreeSize(%d)=(nil,%v); want (_,nil)", treeSize, err)
	}
	fmt.Printf("%s: Got STH(time=%q, size=%d): roothash=%x\n", cfg.Prefix, ctTime(sthN.Timestamp), sthN.TreeSize, sthN.SHA256RootHash)

	// Stage 7: get a consistency proof from 2->(1+N).
	proof2N, err := logClient.GetSTHConsistency(ctx, 2, uint64(treeSize))
	stats.done("GetSTHConsistency", 200)
	if err != nil {
		return fmt.Errorf("got GetSTHConsistency(2, %d)=(nil,%v); want (_,nil)", treeSize, err)
	}
	fmt.Printf("%s: Proof size 2->%d: %x\n", cfg.Prefix, treeSize, proof2N)
	if err := checkCTConsistencyProof(sth2, sthN, proof2N); err != nil {
		return fmt.Errorf("checkCTConsistencyProof(sth2,sthN,proof2N)=%v; want nil", err)
	}

	// Stage 8: get entries [1, N] (start at 1 to skip int-ca.cert)
	entries, err := logClient.GetEntries(ctx, 1, int64(count))
	stats.done("GetEntries", 200)
	if err != nil {
		return fmt.Errorf("got GetEntries(1,%d)=(nil,%v); want (_,nil)", count, err)
	}
	if len(entries) < count {
		return fmt.Errorf("len(entries)=%d; want %d", len(entries), count)
	}
	for i, entry := range entries {
		leaf := entry.Leaf
		ts := leaf.TimestampedEntry
		if leaf.Version != 0 {
			return fmt.Errorf("leaf[%d].Version=%v; want V1(0)", i, leaf.Version)
		}
		if leaf.LeafType != ct.TimestampedEntryLeafType {
			return fmt.Errorf("leaf[%d].Version=%v; want TimestampedEntryLeafType", i, leaf.LeafType)
		}

		if ts.EntryType != ct.X509LogEntryType {
			return fmt.Errorf("leaf[%d].ts.EntryType=%v; want X509LogEntryType", i, ts.EntryType)
		}
		// This assumes that the added entries are sequenced in order.
		// TODO(drysdale): relax this assumption.
		if !bytes.Equal(ts.X509Entry.Data, chain[i+1][0].Data) {
			return fmt.Errorf("leaf[%d].ts.X509Entry differs from originally uploaded cert", i)
		}
	}
	fmt.Printf("%s: Got entries [1:%d+1]\n", cfg.Prefix, count)
	if err := stats.check(cfg); err != nil {
		return fmt.Errorf("unexpected stats check: %v", err)
	}

	// Stage 9: get an audit proof for each certificate we have an SCT for.
	for i := 1; i <= count; i++ {
		sct := scts[i]
		// Calculate leaf hash =  SHA256(0x00 | tls-encode(MerkleTreeLeaf))
		leaf := ct.MerkleTreeLeaf{
			Version:  ct.V1,
			LeafType: ct.TimestampedEntryLeafType,
			TimestampedEntry: &ct.TimestampedEntry{
				Timestamp:  sct.Timestamp,
				EntryType:  ct.X509LogEntryType,
				X509Entry:  &(chain[i][0]),
				Extensions: sct.Extensions,
			},
		}
		leafData, err := tls.Marshal(leaf)
		if err != nil {
			return fmt.Errorf("tls.Marshal(leaf[%d])=(nil,%v); want (_,nil)", i, err)
		}
		hash := sha256.Sum256(append([]byte{merkletree.LeafPrefix}, leafData...))
		rsp, err := logClient.GetProofByHash(ctx, hash[:], sthN.TreeSize)
		stats.done("GetProofByHash", 200)
		if err != nil {
			return fmt.Errorf("got GetProofByHash(sct[%d],size=%d)=(nil,%v); want (_,nil)", i, sthN.TreeSize, err)
		}
		if rsp.LeafIndex != int64(i) {
			return fmt.Errorf("got GetProofByHash(sct[%d],size=%d).LeafIndex=%d; want %d", i, sthN.TreeSize, rsp.LeafIndex, i)
		}
		if err := verifier.VerifyInclusionProof(int64(i), int64(sthN.TreeSize), rsp.AuditPath, sthN.SHA256RootHash[:], leafData); err != nil {
			return fmt.Errorf("got VerifyInclusionProof(%d, %d,...)=%v", i, sthN.TreeSize, err)
		}
	}
	fmt.Printf("%s: Got inclusion proofs [1:%d+1]\n", cfg.Prefix, count)
	if err := stats.check(cfg); err != nil {
		return fmt.Errorf("unexpected stats check: %v", err)
	}

	// Stage 10: attempt to upload a corrupt certificate.
	corruptChain := make([]ct.ASN1Cert, len(chain[1]))
	copy(corruptChain, chain[1])
	corruptAt := len(corruptChain[0].Data) - 3
	corruptChain[0].Data[corruptAt] = (corruptChain[0].Data[corruptAt] + 1)
	if sct, err := logClient.AddChain(ctx, corruptChain); err == nil {
		return fmt.Errorf("got AddChain(corrupt-cert)=(%+v,nil); want (nil,error)", sct)
	}
	stats.done("AddChain", 400)
	fmt.Printf("%s: AddChain(corrupt-cert)=nil,%v\n", cfg.Prefix, err)

	// Stage 11: attempt to upload a certificate without chain.
	if sct, err := logClient.AddChain(ctx, chain[1][0:0]); err == nil {
		return fmt.Errorf("got AddChain(leaf-only)=(%+v,nil); want (nil,error)", sct)
	}
	stats.done("AddChain", 400)
	fmt.Printf("%s: AddChain(leaf-only)=nil,%v\n", cfg.Prefix, err)
	if err := stats.check(cfg); err != nil {
		return fmt.Errorf("unexpected stats check: %v", err)
	}

	// Stage 12: build and add a pre-certificate.
	prechain, tbs, err := makePrecertChain(chain[1], chain[0])
	if err != nil {
		return fmt.Errorf("failed to build pre-certificate: %v", err)
	}
	precertSCT, err := logClient.AddPreChain(ctx, prechain)
	stats.done("AddPreChain", 200)
	if err != nil {
		return fmt.Errorf("got AddPreChain()=(nil,%v); want (_,nil)", err)
	}
	fmt.Printf("%s: Uploaded precert to %v log, got SCT(time=%q)\n", cfg.Prefix, precertSCT.SCTVersion, ctTime(precertSCT.Timestamp))
	treeSize++
	sthN1, err := awaitTreeSize(ctx, logClient, uint64(treeSize), true, &stats)
	if err != nil {
		return fmt.Errorf("awaitTreeSize(%d)=(nil,%v); want (_,nil)", treeSize, err)
	}
	fmt.Printf("%s: Got STH(time=%q, size=%d): roothash=%x\n", cfg.Prefix, ctTime(sthN1.Timestamp), sthN1.TreeSize, sthN1.SHA256RootHash)

	// Stage 13: retrieve and check pre-cert.
	precertIndex := int64(count + 1)
	precertEntries, err := logClient.GetEntries(ctx, precertIndex, precertIndex)
	stats.done("GetEntries", 200)
	if err != nil {
		return fmt.Errorf("got GetEntries(%d,%d)=(nil,%v); want (_,nil)", precertIndex, precertIndex, err)
	}
	if len(precertEntries) != 1 {
		return fmt.Errorf("len(entries)=%d; want %d", len(precertEntries), count)
	}
	leaf := precertEntries[0].Leaf
	ts := leaf.TimestampedEntry
	fmt.Printf("%s: Entry[%d] = {Index:%d Leaf:{Version:%v TS:{EntryType:%v Timestamp:%v}}}\n",
		cfg.Prefix, precertIndex, precertEntries[0].Index, leaf.Version, ts.EntryType, ctTime(ts.Timestamp))

	if ts.EntryType != ct.PrecertLogEntryType {
		return fmt.Errorf("leaf[%d].ts.EntryType=%v; want PrecertLogEntryType", precertIndex, ts.EntryType)
	}

	// This assumes that the added entries are sequenced in order.
	if !bytes.Equal(ts.PrecertEntry.TBSCertificate, tbs) {
		return fmt.Errorf("leaf[%d].ts.PrecertEntry differs from originally uploaded cert", precertIndex)
	}
	if err := stats.check(cfg); err != nil {
		return fmt.Errorf("unexpected stats check: %v", err)
	}

	// Stage 14: get an inclusion proof for the precert.
	// Calculate leaf hash =  SHA256(0x00 | tls-encode(MerkleTreeLeaf))
	issuer, err := x509.ParseCertificate(prechain[1].Data)
	if err != nil {
		return fmt.Errorf("failed to parse issuer certificate: %v", err)
	}
	leaf = ct.MerkleTreeLeaf{
		Version:  ct.V1,
		LeafType: ct.TimestampedEntryLeafType,
		TimestampedEntry: &ct.TimestampedEntry{
			Timestamp: precertSCT.Timestamp,
			EntryType: ct.PrecertLogEntryType,
			PrecertEntry: &ct.PreCert{
				IssuerKeyHash:  sha256.Sum256(issuer.RawSubjectPublicKeyInfo),
				TBSCertificate: tbs,
			},
			Extensions: precertSCT.Extensions,
		},
	}
	leafData, err := tls.Marshal(leaf)
	if err != nil {
		return fmt.Errorf("tls.Marshal(precertLeaf)=(nil,%v); want (_,nil)", err)
	}
	hash := sha256.Sum256(append([]byte{merkletree.LeafPrefix}, leafData...))
	rsp, err := logClient.GetProofByHash(ctx, hash[:], sthN1.TreeSize)
	stats.done("GetProofByHash", 200)
	if err != nil {
		return fmt.Errorf("got GetProofByHash(precertSCT, size=%d)=nil,%v", sthN1.TreeSize, err)
	}
	if want := int64(count + 1); rsp.LeafIndex != want {
		return fmt.Errorf("got GetProofByHash(precertSCT, size=%d).LeafIndex=%d; want %d", sthN1.TreeSize, rsp.LeafIndex, want)
	}
	fmt.Printf("%s: Inclusion proof leaf %d @ %d -> root %d = %x\n", cfg.Prefix, precertIndex, precertSCT.Timestamp, sthN1.TreeSize, rsp.AuditPath)
	if err := verifier.VerifyInclusionProof(precertIndex, int64(sthN1.TreeSize), rsp.AuditPath, sthN1.SHA256RootHash[:], leafData); err != nil {
		return fmt.Errorf("got VerifyInclusionProof(%d,%d,...)=%v; want nil", precertIndex, sthN1.TreeSize, err)
	}
	if err := stats.check(cfg); err != nil {
		return fmt.Errorf("unexpected stats check: %v", err)
	}

	return nil
}

func ctTime(ts uint64) time.Time {
	secs := int64(ts / 1000)
	msecs := int64(ts % 1000)
	return time.Unix(secs, msecs*1000000)
}

func signatureToString(signed *ct.DigitallySigned) string {
	return fmt.Sprintf("Signature: Hash=%v Sign=%v Value=%x", signed.Algorithm.Hash, signed.Algorithm.Signature, signed.Signature)
}

func getChain(path string) ([]ct.ASN1Cert, error) {
	certdata, err := ioutil.ReadFile(filepath.Join(*testDir, path))
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %v", err)
	}
	return testonly.CertsFromPEM(certdata), nil
}

func awaitTreeSize(ctx context.Context, logClient *client.LogClient, size uint64, exact bool, stats *wantStats) (*ct.SignedTreeHead, error) {
	var sth *ct.SignedTreeHead
	for sth == nil || sth.TreeSize < size {
		time.Sleep(200 * time.Millisecond)
		var err error
		sth, err = logClient.GetSTH(ctx)
		stats.done("GetSTH", 200)
		if err != nil {
			return nil, fmt.Errorf("failed to get STH: %v", err)
		}
	}
	if exact && sth.TreeSize != size {
		return nil, fmt.Errorf("sth.TreeSize=%d; want 1", sth.TreeSize)
	}
	return sth, nil
}

func checkCTConsistencyProof(sth1, sth2 *ct.SignedTreeHead, proof [][]byte) error {
	return verifier.VerifyConsistencyProof(int64(sth1.TreeSize), int64(sth2.TreeSize),
		sth1.SHA256RootHash[:], sth2.SHA256RootHash[:], proof)
}

func makePrecertChain(chain, issuerData []ct.ASN1Cert) ([]ct.ASN1Cert, []byte, error) {
	prechain := make([]ct.ASN1Cert, len(chain))
	copy(prechain[1:], chain[1:])
	cert, err := x509.ParseCertificate(chain[0].Data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse certificate to build precert from: %v", err)
	}
	issuer, err := x509.ParseCertificate(issuerData[0].Data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse issuer of precert: %v", err)
	}

	// Add the CT poison extension then rebuild the certificate.
	cert.ExtraExtensions = append(cert.ExtraExtensions, pkix.Extension{
		Id:       x509.OIDExtensionCTPoison,
		Critical: true,
		Value:    []byte{0x05, 0x00}, // ASN.1 NULL
	})

	km, err := crypto.LoadPasswordProtectedPrivateKey(filepath.Join(*testDir, "int-ca.privkey.pem"), "babelfish")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load private key for re-signing: %v", err)
	}
	signer, err := km.Signer()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve signer for re-signing: %v", err)
	}
	prechain[0].Data, err = x509.CreateCertificate(cryptorand.Reader, cert, issuer, cert.PublicKey, signer)

	// Rebuilding the certificate will set the authority key ID to the issuer's subject
	// key ID, and will re-order extensions.  Extract the corresponding TBSCertificate
	// and remove the poison for future reference.
	reparsed, err := x509.ParseCertificate(prechain[0].Data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to re-parse created precertificate: %v", err)
	}
	tbs, err := x509.RemoveCTPoison(reparsed.RawTBSCertificate)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to remove poison from TBSCertificate: %v", err)
	}
	return prechain, tbs, nil
}

// Track HTTP requests/responses in parallel so we can check the stats exported by the log.
type wantStats ctfe.LogStats

func newWantStats(logID int64) wantStats {
	stats := wantStats{
		LogID:       int(logID),
		HTTPAllRsps: make(map[string]int),
		HTTPReq:     make(map[string]int),
		HTTPRsps:    make(map[string]map[string]int),
	}
	for _, ep := range ctfe.Entrypoints {
		stats.HTTPRsps[ep] = make(map[string]int)
	}
	return stats
}

func (want *wantStats) done(ep string, rc int) {
	want.HTTPAllReqs++
	status := strconv.Itoa(rc)
	want.HTTPAllRsps[status]++
	want.HTTPReq[ep]++
	want.HTTPRsps[ep][status]++
}

func (want *wantStats) check(cfg ctfe.LogConfig) error {
	statsURI := "http://" + (*httpServerFlag) + "/debug/vars"
	httpReq, err := http.NewRequest(http.MethodGet, statsURI, nil)
	if err != nil {
		return fmt.Errorf("failed to build GET request: %v", err)
	}
	client := new(http.Client)

	httpRsp, err := ctxhttp.Do(context.Background(), client, httpReq)
	if err != nil {
		return fmt.Errorf("getting stats failed: %v", err)
	}
	defer httpRsp.Body.Close()
	defer ioutil.ReadAll(httpRsp.Body)
	if httpRsp.StatusCode != http.StatusOK {
		return fmt.Errorf("got HTTP Status %q", httpRsp.Status)
	}

	var stats ctfe.AllStats
	if err := json.NewDecoder(httpRsp.Body).Decode(&stats); err != nil {
		return fmt.Errorf("failed to json.Decode() result: %v", err)
	}
	got := stats.Logs[cfg.Prefix]
	if got.LogID != want.LogID {
		return fmt.Errorf("got stats.log-id %d, want %d", got.LogID, want.LogID)
	}
	if got.HTTPAllReqs != want.HTTPAllReqs {
		return fmt.Errorf("got stats.http-all-reqs %d, want %d", got.HTTPAllReqs, want.HTTPAllReqs)
	}
	rcs := []string{"200", "400"}
	for _, rc := range rcs {
		if got.HTTPAllRsps[rc] != want.HTTPAllRsps[rc] {
			return fmt.Errorf("got stats.http-all-rsps[%s]=%d; want %d", rc, got.HTTPAllRsps[rc], want.HTTPAllRsps[rc])
		}
	}
	for _, ep := range ctfe.Entrypoints {
		if got.HTTPReq[ep] != want.HTTPReq[ep] {
			return fmt.Errorf("got stats.http-reqs[%s] %d, want %d", ep, got.HTTPReq[ep], want.HTTPReq[ep])
		}
		for _, rc := range rcs {
			if got.HTTPRsps[ep][rc] != want.HTTPRsps[ep][rc] {
				return fmt.Errorf("got stats.http-rsps[%s][%s]=%d; want %d", ep, rc, got.HTTPRsps[ep][rc], want.HTTPRsps[ep][rc])
			}
		}
	}

	return nil
}
