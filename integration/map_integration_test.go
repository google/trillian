//+build integration

package integration

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"math/rand"
	"testing"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/testonly"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var server = flag.String("server", "localhost:8091", "Server address:port")
var mapID = flag.Int64("map_id", 1, "Trillian MapID to use for test")

func getClient() (*grpc.ClientConn, trillian.TrillianMapClient, error) {
	conn, err := grpc.Dial(*server, grpc.WithInsecure())
	if err != nil {
		return nil, nil, err
	}
	return conn, trillian.NewTrillianMapClient(conn), nil
}

func TestMapIntegration(t *testing.T) {
	conn, client, err := getClient()
	if err != nil {
		t.Fatalf("Failed to get map client: %v", err)
	}
	defer conn.Close()

	{
		// Ensure we're starting with an empty map
		r, err := client.GetSignedMapRoot(context.Background(), &trillian.GetSignedMapRootRequest{*mapID})
		if err != nil {
			t.Fatalf("Failed to get empty map head: %v", err)
		}

		if got, want := r.MapRoot.MapRevision, int64(0); got != want {
			t.Fatalf("Got SMH with revision %d, expected %d", got, want)
		}
	}

	const batchSize = 64
	const numBatches = 32
	const expectedRootB64 = "XxWv/gFSjVVujxdCdDX4Z/GC/9JD8g/y8s1Ayf+boaE="
	expectedKeys := make([][]byte, 0, batchSize*numBatches)
	expectedValues := make(map[string][]byte)

	{
		// Write some data in batches
		rev := int64(0)
		var root trillian.Hash
		for x := 0; x < numBatches; x++ {
			t.Logf("Starting batch %d...", x)

			req := &trillian.SetMapLeavesRequest{
				MapId:    *mapID,
				KeyValue: make([]*trillian.KeyValue, batchSize),
			}

			for y := 0; y < batchSize; y++ {
				key := []byte(fmt.Sprintf("key-%d-%d", x, y))
				expectedKeys = append(expectedKeys, key)
				value := []byte(fmt.Sprintf("value-%d-%d", x, y))
				expectedValues[string(key)] = value
				req.KeyValue[y] = &trillian.KeyValue{
					Key: key,
					Value: &trillian.MapLeaf{
						LeafValue: value,
					},
				}
			}

			resp, err := client.SetLeaves(context.Background(), req)
			if err != nil {
				t.Fatalf("Failed to write batch %d: %v", x, err)
			}
			t.Logf("Set %d k/v pairs", len(req.KeyValue))
			root = resp.MapRoot.RootHash
			rev++
		}
		if expected, got := testonly.MustDecodeBase64(expectedRootB64), root; !bytes.Equal(expected, root) {
			glog.Fatalf("Expected root %s, got root: %s", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
		}
	}

	var latestRoot trillian.SignedMapRoot
	{
		// Check your head
		r, err := client.GetSignedMapRoot(context.Background(), &trillian.GetSignedMapRootRequest{*mapID})
		if err != nil {
			t.Fatalf("Failed to get map head: %v", err)
		}

		if got, want := r.MapRoot.MapRevision, int64(numBatches); got != want {
			t.Fatalf("Got SMH with revision %d, expected %d", got, want)
		}
		if expected, got := testonly.MustDecodeBase64(expectedRootB64), r.MapRoot.RootHash; !bytes.Equal(expected, got) {
			t.Fatalf("Expected root %s, got root: %s", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
		}
		t.Logf("Got expected roothash@%d: %s", r.MapRoot.MapRevision, base64.StdEncoding.EncodeToString(r.MapRoot.RootHash))
		latestRoot = *r.MapRoot
	}

	{
		// Check values
		getReq := trillian.GetMapLeavesRequest{
			MapId:    *mapID,
			Revision: latestRoot.MapRevision,
		}
		// Mix up the ordering of requests
		keyOrder := rand.Perm(len(expectedKeys))
		i := 0

		h := merkle.NewMapHasher(merkle.NewRFC6962TreeHasher(trillian.NewSHA256()))

		for x := 0; x < numBatches; x++ {
			getReq.Key = make([][]byte, 0, batchSize)
			for y := 0; y < batchSize; y++ {
				getReq.Key = append(getReq.Key, expectedKeys[keyOrder[i]])
				i++
			}
			r, err := client.GetLeaves(context.Background(), &getReq)
			if err != nil {
				t.Fatalf("Failed to get values: %v", err)
			}
			if got, want := len(r.KeyValue), len(getReq.Key); got != want {
				t.Errorf("Got %d values, expected %d", got, want)
			}
			for _, kv := range r.KeyValue {
				ev := expectedValues[string(kv.KeyValue.Key)]
				if ev == nil {
					t.Errorf("Unexpected key returned: %v", string(kv.KeyValue.Key))
					continue
				}
				if got, want := ev, kv.KeyValue.Value.LeafValue; !bytes.Equal(got, want) {
					t.Errorf("Got value %x, expected %x", got, want)
					continue
				}
				keyHash := h.HashKey(kv.KeyValue.Key)
				leafHash := h.HashLeaf(kv.KeyValue.Value.LeafValue)
				proof := make([]trillian.Hash, len(kv.Inclusion))
				for i, v := range kv.Inclusion {
					proof[i] = v
				}
				if err := merkle.VerifyMapInclusionProof(keyHash, leafHash, latestRoot.RootHash, proof, h); err != nil {
					t.Errorf("Inclusion proof failed to verify for key %s: %v", kv.KeyValue.Key, err)
				}
				delete(expectedValues, string(kv.KeyValue.Key))
			}
		}
		if got := len(expectedValues); got != 0 {
			t.Fatalf("Still have %d unmatched expected values remaining", got)
		}

	}

}
