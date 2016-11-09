//+build integration

package integration

import (
	"flag"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	ct "github.com/google/certificate-transparency/go"
	"github.com/google/certificate-transparency/go/client"
	"github.com/google/certificate-transparency/go/jsonclient"
	"golang.org/x/net/context"
)

var httpServerFlag = flag.String("ct_http_server", "localhost:8092", "Server address:port")
var pubKey = flag.String("public_key_file", "", "Name of file containing log's public key")

func TestCTIntegration(t *testing.T) {
	flag.Parse()

	logURI := "http://" + (*httpServerFlag)

	opts := jsonclient.Options{}
	if *pubKey != "" {
		pubkey, err := ioutil.ReadFile(*pubKey)
		if err != nil {
			t.Fatalf("Failed to get public key contents: ", err)
		}
		opts.PublicKey = string(pubkey)
	}
	logClient, err := client.New(logURI, nil, opts)
	if err != nil {
		t.Fatalf("Failed to create LogClient instance: ", err)
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
	sth, err := logClient.GetSTH()
	if err != nil {
		t.Fatalf("Failed to get STH: %v", err)
	}
	if sth.Version != 0 {
		t.Errorf("sth.Version=%v; want V1(0)", sth.Version)
	}
	if sth.TreeSize != 0 {
		t.Errorf("sth.TreeSize=%d; want 0", sth.TreeSize)
	}
	when := ctTimestampToTime(sth.Timestamp)
	fmt.Printf("%v: Got STH: %x\n", when, sth.Version, sth.TreeSize, sth.SHA256RootHash)
	fmt.Printf("%v\n", signatureToString(&sth.TreeHeadSignature))

	// Stage 2: add a cert, get an SCT
	// TODO(drysdale)

	// Stage 3: keep getting the STH until tree size becomes 1.
	// TODO(drysdale)

	// Stage 4: get a consistency proof from 0->1.
	// TODO(drysdale)

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

func ctTimestampToTime(ts uint64) time.Time {
	secs := int64(ts / 1000)
	msecs := int64(ts % 1000)
	return time.Unix(secs, msecs*1000000)
}

func signatureToString(signed *ct.DigitallySigned) string {
	return fmt.Sprintf("Signature: Hash=%v Sign=%v Value=%x", signed.Algorithm.Hash, signed.Algorithm.Signature, signed.Signature)
}
