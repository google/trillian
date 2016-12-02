//+build integration

package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/client"
	"github.com/google/certificate-transparency/go/jsonclient"
	"github.com/google/certificate-transparency/go/merkletree"
	"github.com/google/certificate-transparency/go/tls"
	"golang.org/x/net/context"
)

var httpServerFlag = flag.String("ct_http_server", "localhost:8092", "Server address:port")
var pubKey = flag.String("public_key_file", "", "Name of file containing log's public key")
var testdata = flag.String("testdata", "testdata", "Name of directory with test data")
var seed = flag.Int64("seed", -1, "Seed for random number generation")

var verifier = merkletree.NewMerkleVerifier(func(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
})

func TestCTIntegration(t *testing.T) {
	flag.Parse()
	logURI := "http://" + (*httpServerFlag)
	if *seed == -1 {
		*seed = time.Now().UTC().UnixNano() & 0xFFFFFFFF
	}
	fmt.Printf("Today's test has been brought to you by the letters C and T and the number %#x\n", *seed)
	rand.Seed(*seed)

	opts := jsonclient.Options{}
	if *pubKey != "" {
		pubkey, err := ioutil.ReadFile(*pubKey)
		if err != nil {
			t.Fatalf("Failed to get public key contents: %v", err)
		}
		opts.PublicKey = string(pubkey)
	}
	logClient, err := client.New(logURI, nil, opts)
	if err != nil {
		t.Fatalf("Failed to create LogClient instance: %v", err)
	}
	ctx := context.Background()

	// Stage 0: get accepted roots, which should just be the fake CA.
	roots, err := logClient.GetAcceptedRoots(ctx)
	if err != nil {
		t.Fatalf("Failed to get roots: %v", err)
	}
	if len(roots) != 1 {
		t.Errorf("len(GetAcceptableRoots())=%d; want 1", len(roots))
	}

	// Stage 1: get the STH, which should be empty.
	sth0, err := logClient.GetSTH(ctx)
	if err != nil {
		t.Fatalf("Failed to get STH: %v", err)
	}
	if sth0.Version != 0 {
		t.Errorf("sth.Version=%v; want V1(0)", sth0.Version)
	}
	if sth0.TreeSize != 0 {
		t.Fatalf("sth.TreeSize=%d; want 0", sth0.TreeSize)
	}
	fmt.Printf("%v: Got STH(size=%d): roothash=%x\n", ctTime(sth0.Timestamp), sth0.TreeSize, sth0.SHA256RootHash)

	// Stage 2: add a single cert (the intermediate CA), get an SCT.
	var scts [21]*ct.SignedCertificateTimestamp // 0=int-ca, 1-20=leaves
	var chain [21][]ct.ASN1Cert
	chain[0], err = getChain("int-ca.cert")
	if err != nil {
		t.Fatalf("Failed to load certificate: %v", err)
	}
	scts[0], err = logClient.AddChain(ctx, chain[0])
	if err != nil {
		t.Fatalf("Failed to AddChain(): %v", err)
	}
	// Display the SCT
	fmt.Printf("%v: Uploaded int-ca.cert to %v log, got SCT\n", ctTime(scts[0].Timestamp), scts[0].SCTVersion)

	// Keep getting the STH until tree size becomes 1.
	sth1, err := awaitTreeSize(ctx, logClient, 1, true)
	if err != nil {
		t.Fatalf("Failed to get STH for size=1: %v", err)
	}
	fmt.Printf("%v: Got STH(size=%d): roothash=%x\n", ctTime(sth1.Timestamp), sth1.TreeSize, sth1.SHA256RootHash)

	// Stage 3: add a second cert, wait for tree size = 2
	chain[1], err = getChain("leaf01.chain")
	if err != nil {
		t.Fatalf("Failed to load certificate: %v", err)
	}
	scts[1], err = logClient.AddChain(ctx, chain[1])
	if err != nil {
		t.Fatalf("Failed to AddChain(): %v", err)
	}
	fmt.Printf("%v: Uploaded cert01.chain to %v log, got SCT\n", ctTime(scts[1].Timestamp), scts[1].SCTVersion)
	sth2, err := awaitTreeSize(ctx, logClient, 2, true)
	if err != nil {
		t.Fatalf("Failed to get STH for size=1: %v", err)
	}
	fmt.Printf("%v: Got STH(size=%d): roothash=%x\n", ctTime(sth2.Timestamp), sth2.TreeSize, sth2.SHA256RootHash)

	// Stage 4: get a consistency proof from size 1-> size 2.
	proof12, err := logClient.GetSTHConsistency(ctx, 1, 2)
	if err != nil {
		t.Fatalf("Failed to GetSTHConsistency(1, 2): %v", err)
	}
	//                 sth2
	//                 / \
	//  sth1   =>      a b
	//    |            | |
	//   d0           d0 d1
	// So consistency proof is [b] and we should have:
	//   sth2 == SHA256(0x01 | sth1 | b)
	if len(proof12) != 1 {
		t.Fatalf("len(proof12)=%d; want 1", len(proof12))
	}
	if err := checkCTConsistencyProof(sth1, sth2, proof12); err != nil {
		t.Fatalf("consistency proof verification failed: %v", err)
	}

	// Stage 5: add certificates 2, 3, 4, 5,...N, for some random N in [4,20]
	atLeast := 4
	count := atLeast + rand.Intn(20-atLeast)
	for i := 2; i <= count; i++ {
		filename := fmt.Sprintf("leaf%02d.chain", i)
		chain[i], err = getChain(filename)
		if err != nil {
			t.Errorf("Failed to load certificate: %v", err)
		}
		scts[i], err = logClient.AddChain(ctx, chain[i])
		if err != nil {
			t.Fatalf("Failed to AddChain(): %v", err)
		}
		fmt.Printf("%v: Uploaded %s to %v log, got SCT[%d]\n", ctTime(scts[i].Timestamp), filename, scts[i].SCTVersion, i)
	}

	// Stage 6: keep getting the STH until tree size becomes 1 + N (allows for int-ca.cert).
	treeSize := 1 + count
	sthN, err := awaitTreeSize(ctx, logClient, uint64(treeSize), true)
	if err != nil {
		t.Fatalf("Failed to get STH for size=%d: %v", treeSize, err)
	}
	fmt.Printf("%v: Got STH(size=%d): roothash=%x\n", ctTime(sthN.Timestamp), sthN.TreeSize, sthN.SHA256RootHash)

	// Stage 7: get a consistency proof from 2->(1+N).
	proof2N, err := logClient.GetSTHConsistency(ctx, 2, uint64(treeSize))
	if err != nil {
		t.Errorf("Failed to GetSTHConsistency(2, %d): %v", treeSize, err)
	} else {
		fmt.Printf("Proof size 2->%d: %x\n", treeSize, proof2N)
		if err := checkCTConsistencyProof(sth2, sthN, proof2N); err != nil {
			t.Errorf("consistency proof verification failed: %v", err)
		}
	}

	// Stage 8: get entries [1, N] (start at 1 to skip int-ca.cert)
	entries, err := logClient.GetEntries(ctx, 1, int64(count))
	if err != nil {
		t.Errorf("Failed to GetEntries(1, %d): %v", count, err)
	} else {
		if len(entries) < count {
			t.Errorf("Fewer entries (%d) retrieved than expected (%d)", len(entries), count)
		}
		for i, entry := range entries {
			leaf := entry.Leaf
			ts := leaf.TimestampedEntry
			fmt.Printf("Entry[%d] = {Index:%d Leaf:{Version:%v TS:{EntryType:%v Timestamp:%v}}}\n", 1+i, entry.Index, leaf.Version, ts.EntryType, ctTime(ts.Timestamp))
			if leaf.Version != 0 {
				t.Errorf("leaf[%d].Version=%v; want V1(0)", i, leaf.Version)
			}
			if leaf.LeafType != ct.TimestampedEntryLeafType {
				t.Errorf("leaf[%d].Version=%v; want TimestampedEntryLeafType", i, leaf.LeafType)
			}

			if ts.EntryType != ct.X509LogEntryType {
				t.Errorf("leaf[%d].ts.EntryType=%v; want X509LogEntryType", i, ts.EntryType)
				continue
			}
			// This assumes that the added entries are sequenced in order.
			if !bytes.Equal(ts.X509Entry.Data, chain[i+1][0].Data) {
				t.Errorf("leaf[%d].ts.X509Entry differs from originally uploaded cert", i)
				t.Errorf("\tuploaded:  %s", hex.EncodeToString(chain[i+1][0].Data))
				t.Errorf("\tretrieved: %s", hex.EncodeToString(ts.X509Entry.Data))
			}
		}
	}

	// Stage 9: get an audit proof for each certificate we have an SCT for.
	for i := 1; i <= count; i++ {
		sct := scts[i]
		fmt.Printf("Inclusion proof leaf %d @ %d -> root %d = ", i, sct.Timestamp, sthN.TreeSize)
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
			fmt.Printf("<fail: %v>\n", err)
			t.Errorf("tls.Marshal(leaf[%d])=nil,%v", i, err)
			continue
		}
		hash := sha256.Sum256(append([]byte{merkletree.LeafPrefix}, leafData...))
		rsp, err := logClient.GetProofByHash(ctx, hash[:], sthN.TreeSize)
		if err != nil {
			fmt.Printf("<fail: %v>\n", err)
			t.Errorf("GetProofByHash(sct[%d], size=%d)=nil,%v", i, sthN.TreeSize, err)
			continue
		}
		if rsp.LeafIndex != int64(i) {
			fmt.Printf("<fail: wrong index>\n", err)
			t.Errorf("GetProofByHash(sct[%d], size=%d) has LeafIndex %d", i, sthN.TreeSize, rsp.LeafIndex)
			continue
		}
		fmt.Printf("%x\n", rsp.AuditPath)
		if err := verifier.VerifyInclusionProof(int64(i), int64(sthN.TreeSize), rsp.AuditPath, sthN.SHA256RootHash[:], leafData); err != nil {
			t.Errorf("inclusion proof verification failed: %v", err)
		}
	}

	// Stage 10: attempt to upload a corrupt certificate.
	corruptChain := make([]ct.ASN1Cert, len(chain[1]))
	copy(corruptChain, chain[1])
	corruptAt := len(corruptChain[0].Data) - 3
	corruptChain[0].Data[corruptAt] = (corruptChain[0].Data[corruptAt] + 1)
	if sct, err := logClient.AddChain(ctx, corruptChain); err == nil {
		t.Fatalf("AddChain(corrupt-cert)=%+v,nil; want error", sct)
	} else {
		fmt.Printf("AddChain(corrupt-cert)=nil,%v\n", err)
	}

	// Stage 11: attempt to upload a certificate without chain.
	if sct, err := logClient.AddChain(ctx, chain[1][0:0]); err == nil {
		t.Fatalf("AddChain(leaf-only)=%+v,nil; want error", sct)
	} else {
		fmt.Printf("AddChain(leaf-only)=nil,%v\n", err)
	}
}

func ctTime(ts uint64) time.Time {
	secs := int64(ts / 1000)
	msecs := int64(ts % 1000)
	return time.Unix(secs, msecs*1000000)
}

func signatureToString(signed *ct.DigitallySigned) string {
	return fmt.Sprintf("Signature: Hash=%v Sign=%v Value=%x", signed.Algorithm.Hash, signed.Algorithm.Signature, signed.Signature)
}

func certsFromPEM(data []byte) []ct.ASN1Cert {
	var chain []ct.ASN1Cert
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			chain = append(chain, ct.ASN1Cert{Data: block.Bytes})
		}
	}
	return chain
}

func getChain(path string) ([]ct.ASN1Cert, error) {
	certdata, err := ioutil.ReadFile(filepath.Join(*testdata, path))
	if err != nil {
		return nil, fmt.Errorf("Failed to load certificate: %v", err)
	}
	return certsFromPEM(certdata), nil
}

func awaitTreeSize(ctx context.Context, logClient *client.LogClient, size uint64, exact bool) (*ct.SignedTreeHead, error) {
	var sth *ct.SignedTreeHead
	for sth == nil || sth.TreeSize < size {
		time.Sleep(200 * time.Millisecond)
		var err error
		sth, err = logClient.GetSTH(ctx)
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
