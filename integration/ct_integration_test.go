//+build integration

package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/client"
	"github.com/google/certificate-transparency/go/jsonclient"
	"golang.org/x/net/context"
)

var httpServerFlag = flag.String("ct_http_server", "localhost:8092", "Server address:port")
var pubKey = flag.String("public_key_file", "", "Name of file containing log's public key")
var testdata = flag.String("testdata", "testdata", "Name of directory with test data")

func TestCTIntegration(t *testing.T) {
	flag.Parse()

	logURI := "http://" + (*httpServerFlag)

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
	sth, err := logClient.GetSTH(ctx)
	if err != nil {
		t.Fatalf("Failed to get STH: %v", err)
	}
	if sth.Version != 0 {
		t.Errorf("sth.Version=%v; want V1(0)", sth.Version)
	}
	if sth.TreeSize != 0 {
		t.Errorf("sth.TreeSize=%d; want 0", sth.TreeSize)
	}
	fmt.Printf("%v: Got STH(size=%d): roothash=%x\n", ctTime(sth.Timestamp), sth.TreeSize, sth.SHA256RootHash)

	// Stage 2: add a single cert (the intermediate CA), get an SCT.
	chain, err := getChain("int-ca.cert")
	if err != nil {
		t.Fatalf("Failed to load certificate: %v", err)
	}
	sct, err := logClient.AddChain(ctx, chain)
	if err != nil {
		t.Fatalf("Failed to AddChain(): %v", err)
	}
	// Display the SCT
	fmt.Printf("%v: Uploaded cert to %v log, got SCT\n", ctTime(sct.Timestamp), sct.SCTVersion)

	// Keep getting the STH until tree size becomes 1.
	sth1, err := awaitTreeSize(ctx, logClient, 1, true)
	if err != nil {
		t.Fatalf("Failed to get STH for size=1: %v", err)
	}

	// Stage 3: add a second cert, wait for tree size = 2
	chain, err = getChain("leaf01.chain")
	if err != nil {
		t.Fatalf("Failed to load certificate: %v", err)
	}
	sct, err = logClient.AddChain(ctx, chain)
	if err != nil {
		t.Fatalf("Failed to AddChain(): %v", err)
	}
	fmt.Printf("%v: Uploaded cert to %v log, got SCT\n", ctTime(sct.Timestamp), sct.SCTVersion)
	sth2, err := awaitTreeSize(ctx, logClient, 2, true)
	if err != nil {
		t.Fatalf("Failed to get STH for size=1: %v", err)
	}

	// Stage 4: get a consistency proof from size 1-> size 2.
	proof, err := logClient.GetSTHConsistency(ctx, 1, 2)
	if err != nil {
		t.Fatalf("Failed to GetSTHConsistency(): %v", err)
	}
	//                 sth2
	//                 / \
	//  sth1   =>      a b
	//    |            | |
	//   d0           d0 d1
	// So consistency proof is [b] and we should have:
	//   sth2 == SHA256(0x01 | sth1 | b)
	if len(proof) != 1 {
		t.Fatalf("len(proof)=%d; want 1", len(proof))
	}
	if len(proof[0]) != sha256.Size {
		t.Fatalf("len(proof[0])=%d; want %d", len(proof[0]), sha256.Size)
	}
	fmt.Printf("Proof 1->2 = %x\n", proof)
	data := make([]byte, 1+sha256.Size+sha256.Size)
	data[0] = 0x01
	copy(data[1:1+sha256.Size], sth1.SHA256RootHash[:])
	copy(data[1+sha256.Size:], proof[0])
	if want, got := sha256.Sum256(data), sth2.SHA256RootHash; got != want {
		t.Errorf("SHA256(1|sth1|proof[0])=%s; want %s", hex.EncodeToString(got[:]), hex.EncodeToString(want[:]))
	}

	// Stage 5: add certificates 2, 3, 4, 5,...N, for some random N in [4,25]
	// TODO(drysdale)

	// Stage 6: keep getting the STH until tree size becomes N.
	// TODO(drysdale)

	// Stage 7: get a consistency proof from 1->N.
	// TODO(drysdale)

	// Stage 8: get entries [1, N]
	// TODO(drysdale)

	// Stage 9: get an audit proof for cert M, randomly chosen in [1,N]
	// TODO(drysdale)
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
	fmt.Printf("%v: Got STH(size=%d): roothash=%x\n", ctTime(sth.Timestamp), sth.TreeSize, sth.SHA256RootHash)
	return sth, nil
}
