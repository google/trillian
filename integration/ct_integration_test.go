//+build integration

package integration

import (
	"crypto/sha256"
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
	"golang.org/x/net/context"
)

var httpServerFlag = flag.String("ct_http_server", "localhost:8092", "Server address:port")
var pubKey = flag.String("public_key_file", "", "Name of file containing log's public key")
var testdata = flag.String("testdata", "testdata", "Name of directory with test data")
var seed = flag.Int64("seed", -1, "Seed for random number generation")

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
		t.Errorf("sth.TreeSize=%d; want 0", sth0.TreeSize)
	}
	fmt.Printf("%v: Got STH(size=%d): roothash=%x\n", ctTime(sth0.Timestamp), sth0.TreeSize, sth0.SHA256RootHash)

	// Stage 2: add a single cert (the intermediate CA), get an SCT.
	chain0, err := getChain("int-ca.cert")
	if err != nil {
		t.Fatalf("Failed to load certificate: %v", err)
	}
	sct, err := logClient.AddChain(ctx, chain0)
	if err != nil {
		t.Fatalf("Failed to AddChain(): %v", err)
	}
	// Display the SCT
	fmt.Printf("%v: Uploaded int-ca.cert to %v log, got SCT\n", ctTime(sct.Timestamp), sct.SCTVersion)

	// Keep getting the STH until tree size becomes 1.
	sth1, err := awaitTreeSize(ctx, logClient, 1, true)
	if err != nil {
		t.Fatalf("Failed to get STH for size=1: %v", err)
	}
	fmt.Printf("%v: Got STH(size=%d): roothash=%x\n", ctTime(sth1.Timestamp), sth1.TreeSize, sth1.SHA256RootHash)

	// Stage 3: add a second cert, wait for tree size = 2
	chain1, err := getChain("leaf01.chain")
	if err != nil {
		t.Fatalf("Failed to load certificate: %v", err)
	}
	sct, err = logClient.AddChain(ctx, chain1)
	if err != nil {
		t.Fatalf("Failed to AddChain(): %v", err)
	}
	fmt.Printf("%v: Uploaded cert01.chain to %v log, got SCT\n", ctTime(sct.Timestamp), sct.SCTVersion)
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
		chain, err := getChain(filename)
		if err != nil {
			t.Errorf("Failed to load certificate: %v", err)
		}
		sct, err = logClient.AddChain(ctx, chain)
		if err != nil {
			t.Fatalf("Failed to AddChain(): %v", err)
		}
		fmt.Printf("%v: Uploaded %s to %v log, got SCT\n", ctTime(sct.Timestamp), filename, sct.SCTVersion)
	}

	// Stage 6: keep getting the STH until tree size becomes 1 + N (allows for int-ca.cert).
	count++
	sthN, err := awaitTreeSize(ctx, logClient, uint64(count), true)
	if err != nil {
		t.Fatalf("Failed to get STH for size=%d: %v", count, err)
	}
	fmt.Printf("%v: Got STH(size=%d): roothash=%x\n", ctTime(sthN.Timestamp), sthN.TreeSize, sthN.SHA256RootHash)

	// Stage 7: get a consistency proof from 2->N.
	proof2N, err := logClient.GetSTHConsistency(ctx, 2, uint64(count))
	if err != nil {
		t.Fatalf("Failed to GetSTHConsistency(2, %d): %v", count, err)
	}
	fmt.Printf("Proof 2->%d: %x\n", count, proof2N)
	if err := checkCTConsistencyProof(sth2, sthN, proof2N); err != nil {
		t.Fatalf("consistency proof verification failed: %v", err)
	}

	// Stage 8: get entries [1, N]
	// TODO(drysdale)

	// Stage 9: get an audit proof for cert M, randomly chosen in [1,N]
	// TODO(drysdale)

	// Stage 10: attempt to upload a corrupt certificate.
	corruptAt := len(chain1[0].Data) - 3
	chain1[0].Data[corruptAt] = (chain1[0].Data[corruptAt] + 1)
	sct, err = logClient.AddChain(ctx, chain1)
	if err == nil {
		t.Fatalf("AddChain(corrupt-cert)=%+v,nil; want error", sct)
	}
	fmt.Printf("AddChain(corrupt-cert)=nil,%v\n", err)
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
	verifier := merkletree.NewMerkleVerifier(func(data []byte) []byte {
		hash := sha256.Sum256(data)
		return hash[:]
	})
	return verifier.VerifyConsistencyProof(int64(sth1.TreeSize), int64(sth2.TreeSize),
		sth1.SHA256RootHash[:], sth2.SHA256RootHash[:], proof)
}
