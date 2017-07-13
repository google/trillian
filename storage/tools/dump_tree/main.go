// Copyright 2017 Google Inc. All Rights Reserved.
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

// The dump_tree program uses the in memory storage implementation to create a sequenced
// log tree of a particular size using known leaf data and then dumps out the resulting
// SubTree protos for examination and debugging. It does not require any actual storage
// to be configured.
//
// Examples of some usages:
//
// Print a summary of the storage protos in a tree of size 1044, rebuilding internal nodes:
// dump_tree -tree_size 1044 -summary
//
// Print all versions of all raw subtree protos for a tree of size 58:
// dump_tree -tree_size 58 -latest_version=false
//
// Print the latest revision of each subtree proto for a tree of size 127 with hex keys:
// dump_tree -tree_size 127
//
// Print out the nodes by level using the NodeReader API for a tree of size 11:
// dump_tree -tree_size 11 -traverse
//
// The format for recordio output is as defined in:
// https://github.com/google/or-tools/blob/master/ortools/base/recordio.h
// This program always outputs uncompressed records.
package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	tc "github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/log"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/memory"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/util"
)

// A 32 bit magic number that is written at the start of record io files to identify the format.
const recordIOMagic int32 = 0x3ed7230a

var (
	treeSizeFlag        = flag.Int("tree_size", 871, "The number of leaves to be added to the tree")
	batchSizeFlag       = flag.Int("batch_size", 50, "The batch size for sequencing")
	leafDataFormatFlag  = flag.String("leaf_format", "Leaf %d", "The format string for leaf data")
	latestRevisionFlag  = flag.Bool("latest_version", true, "If true outputs only the latest revision per subtree")
	summaryFlag         = flag.Bool("summary", false, "If true outputs a brief summary per subtree, false dumps the whole proto")
	hexKeysFlag         = flag.Bool("hex_keys", false, "If true shows proto keys as hex rather than base64")
	leafHashesFlag      = flag.Bool("leaf_hashes", false, "If true the summary output includes leaf hashes")
	recordIOFlag        = flag.Bool("recordio", false, "If true outputs in recordio format")
	rebuildInternalFlag = flag.Bool("rebuild", true, "If true rebuilds internal nodes + root hash from leaves")
	traverseFlag        = flag.Bool("traverse", false, "If true dumps a tree traversal via coord space, else raw subtrees")
	dumpLeavesFlag      = flag.Bool("dump_leaves", false, "If true dumps the leaf data from the tree via the API")
)

type treeandrev struct {
	fullKey  string
	subtree  *storagepb.SubtreeProto
	revision int
}

func summarizeProto(s *storagepb.SubtreeProto) string {
	summary := fmt.Sprintf("p: %-20s d: %d lc: %3d ic: %3d rh:%s\n",
		hex.EncodeToString(s.Prefix),
		s.Depth,
		len(s.Leaves),
		s.InternalNodeCount,
		hex.EncodeToString(s.RootHash))

	if *leafHashesFlag {
		for prefix, hash := range s.Leaves {
			dp, err := base64.StdEncoding.DecodeString(prefix)
			if err != nil {
				glog.Fatalf("Failed to decode leaf prefix: %v", err)
			}
			summary += fmt.Sprintf("%s -> %s\n", hex.EncodeToString(dp), hex.EncodeToString(hash))
		}
	}

	return summary
}

func fullProto(s *storagepb.SubtreeProto) string {
	return fmt.Sprintf("%s\n", proto.MarshalTextString(s))
}

func recordIOProto(s *storagepb.SubtreeProto) string {
	buf := new(bytes.Buffer)
	data, err := proto.Marshal(s)
	if err != nil {
		glog.Fatalf("Failed to marshal subtree proto: %v", err)
	}
	dataLen := int64(len(data))
	err = binary.Write(buf, binary.BigEndian, dataLen)
	if err != nil {
		glog.Fatalf("binary.Write failed: %v", err)
	}
	var compLen int64
	err = binary.Write(buf, binary.BigEndian, compLen)
	if err != nil {
		glog.Fatalf("binary.Write failed: %v", err)
	}
	buf.Write(data)

	return buf.String()
}

// This is a copy of the logserver private key from the testdata directory
var logPrivKeyPEM = `
-----BEGIN EC PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: DES-CBC,D95ECC664FF4BDEC

Xy3zzHFwlFwjE8L1NCngJAFbu3zFf4IbBOCsz6Fa790utVNdulZncNCl2FMK3U2T
sdoiTW8ymO+qgwcNrqvPVmjFRBtkN0Pn5lgbWhN/aK3TlS9IYJ/EShbMUzjgVzie
S9+/31whWcH/FLeLJx4cBzvhgCtfquwA+s5ojeLYYsk=
-----END EC PRIVATE KEY-----`

// And the corresponding public key
var logPubKeyPEM = `
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEywnWicNEQ8bn3GXcGpA+tiU4VL70
Ws9xezgQPrg96YGsFrF6KYG68iqyHDlQ+4FWuKfGKXHn3ooVtB/pfawb5Q==
-----END PUBLIC KEY-----`

func sequence(treeID int64, seq *log.Sequencer, count int) {
	glog.Infof("Sequencing batch of size %d", count)
	sequenced, err := seq.SequenceBatch(context.TODO(), treeID, *batchSizeFlag, time.Microsecond, 24*time.Hour)

	if err != nil {
		glog.Fatalf("SequenceBatch got: %v, want: nil", err)
	}

	if got, want := sequenced, count; got != want {
		glog.Fatalf("SequenceBatch got: %d sequenced, want: %d", got, want)
	}
}

func getPrivateKey(pemPath, pemPassword string) (*any.Any, crypto.Signer) {
	pemSigner, err := keys.NewFromPrivatePEM(pemPath, pemPassword)
	if err != nil {
		glog.Fatalf("NewFromPrivatePEM(): %v", err)
	}
	pemDer, err := keys.MarshalPrivateKey(pemSigner)
	if err != nil {
		glog.Fatalf("MarshalPrivateKey(): %v", err)
	}
	anyPrivKey, err := ptypes.MarshalAny(&keyspb.PrivateKey{Der: pemDer})
	if err != nil {
		glog.Fatalf("MarshalAny(%v): %v", pemDer, err)
	}

	return anyPrivKey, pemSigner
}

func getPublicKey(pem string) []byte {
	key, err := keys.NewFromPublicPEM(pem)
	if err != nil {
		panic(err)
	}

	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		panic(err)
	}
	return der
}

func createTree(as storage.AdminStorage) (*trillian.Tree, crypto.Signer) {
	atx, err := as.Begin(context.TODO())
	if err != nil {
		glog.Fatalf("Begin admin TX: %v", err)
	}

	privKey, cSigner := getPrivateKey(logPrivKeyPEM, "towel")
	pubKey := getPublicKey(logPubKeyPEM)

	tree := trillian.Tree{
		TreeType:           trillian.TreeType_LOG,
		TreeState:          trillian.TreeState_ACTIVE,
		HashAlgorithm:      sigpb.DigitallySigned_SHA256,
		HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
		SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
		PrivateKey:         privKey,
		PublicKey:          &keyspb.PublicKey{Der: pubKey},
		MaxRootDuration:    ptypes.DurationProto(0 * time.Millisecond),
	}
	t, err := atx.CreateTree(context.TODO(), &tree)
	if err != nil {
		glog.Fatalf("Create tree: %v", err)
	}
	if err := atx.Commit(); err != nil {
		glog.Fatalf("Commit admin TX: %v", err)
	}

	return t, cSigner
}

func main() {
	flag.Parse()
	validateFlagsOrDie()

	glog.Info("Initializing memory log storage")
	ls := memory.NewLogStorage(monitoring.InertMetricFactory{})
	as := memory.NewAdminStorage(ls)
	tree, cSigner := createTree(as)

	seq := log.NewSequencer(rfc6962.DefaultHasher,
		util.SystemTimeSource{},
		ls,
		&tc.Signer{Signer: cSigner, Hash: crypto.SHA256},
		nil,
		quota.Noop())

	// Create the initial tree head at size 0, which is required. And then sequence the leaves.
	sequence(tree.TreeId, seq, 0)
	sequenceLeaves(ls, seq, tree.TreeId)

	// Handle anything left over
	left := *treeSizeFlag % *batchSizeFlag
	if left == 0 {
		left = *batchSizeFlag
	}
	sequence(tree.TreeId, seq, left)

	// Read the latest STH back
	tx, err := ls.BeginForTree(context.TODO(), tree.TreeId)
	if err != nil {
		glog.Fatalf("BeginForTree got: %v, want: nil", err)
	}

	sth, err := tx.LatestSignedLogRoot(context.TODO())
	if err != nil {
		glog.Fatalf("LatestSignedLogRoot: %v", err)
	}

	glog.Infof("STH at size %d has hash %s@%d",
		sth.TreeSize,
		hex.EncodeToString(sth.RootHash),
		sth.TreeRevision)

	if err := tx.Commit(); err != nil {
		glog.Fatalf("TX commit: %v", err)
	}

	// All leaves are now sequenced into the tree. The current state is what we need.
	glog.Info("Producing output")

	if *traverseFlag {
		traverseTreeStorage(ls, tree.TreeId, *treeSizeFlag, sth.TreeRevision)
		return
	}

	if *dumpLeavesFlag {
		dumpLeaves(ls, tree.TreeId, *treeSizeFlag)
		return
	}

	of := fullProto
	if *summaryFlag {
		of = summarizeProto
	}
	if *recordIOFlag {
		of = recordIOProto
		recordIOHdr()
	}

	hasher, err := hashers.NewLogHasher(trillian.HashStrategy_RFC6962_SHA256)
	if err != nil {
		glog.Fatalf("Failed to create a log hasher: %v", err)
	}
	repopFunc := cache.PopulateLogSubtreeNodes(hasher)

	if *latestRevisionFlag {
		latestRevisions(ls, tree.TreeId, repopFunc, of)
	} else {
		allRevisions(ls, tree.TreeId, repopFunc, of)
	}
}

func allRevisions(ls storage.LogStorage, treeID int64, repopFunc storage.PopulateSubtreeFunc, of func(*storagepb.SubtreeProto) string) {
	memory.DumpSubtrees(ls, treeID, func(k string, v *storagepb.SubtreeProto) {
		if *rebuildInternalFlag {
			repopFunc(v)
		}
		if *hexKeysFlag {
			hexKeys(v)
		}
		fmt.Print(of(v))
	})
}

func latestRevisions(ls storage.LogStorage, treeID int64, repopFunc storage.PopulateSubtreeFunc, of func(*storagepb.SubtreeProto) string) {
	vMap := make(map[string]treeandrev)
	memory.DumpSubtrees(ls, treeID, func(k string, v *storagepb.SubtreeProto) {
		// Relies on the btree key space for subtrees being /tree_id/subtree/<id>/<revision>
		pieces := strings.Split(k, "/")
		if got, want := len(pieces), 5; got != want {
			glog.Fatalf("Wrong no of Btree subtree key segments. Got: %d, want: %d", got, want)
		}

		e := vMap[pieces[3]]
		rev, err := strconv.Atoi(pieces[4])
		if err != nil {
			glog.Fatalf("Bad subtree key: %v", k)
		}

		if rev > e.revision {
			vMap[pieces[3]] = treeandrev{
				fullKey:  k,
				subtree:  v,
				revision: rev,
			}
		}
	})

	// The map should now contain the latest revisions per subtree
	for _, v := range vMap {
		if *rebuildInternalFlag {
			repopFunc(v.subtree)
		}
		if *hexKeysFlag {
			hexKeys(v.subtree)
		}
		fmt.Print(of(v.subtree))
	}
}

func validateFlagsOrDie() {
	if *summaryFlag && *recordIOFlag {
		glog.Fatal("-summary and -recordio are mutually exclusive flags")
	}
}

func sequenceLeaves(ls storage.LogStorage, seq *log.Sequencer, treeID int64) {
	glog.Info("Queuing work")
	for l := 0; l < *treeSizeFlag; l++ {
		glog.V(1).Infof("Queuing leaf %d", l)

		leafData := []byte(fmt.Sprintf(*leafDataFormatFlag, l))
		tx, err := ls.BeginForTree(context.TODO(), treeID)
		if err != nil {
			glog.Fatalf("BeginForTree got: %v, want: nil", err)
		}

		hash := sha256.Sum256(leafData)
		lh := []byte(hash[:])
		leaf := trillian.LogLeaf{LeafValue: leafData, LeafIdentityHash: lh, MerkleLeafHash: lh}
		leaves := []*trillian.LogLeaf{&leaf}

		if _, err := tx.QueueLeaves(context.TODO(), leaves, time.Now()); err != nil {
			glog.Fatalf("QueueLeaves got: %v, want: nil", err)
		}

		if err := tx.Commit(); err != nil {
			glog.Fatalf("Queueleaves TX commit: %v", err)
		}

		if l > 0 && l%*batchSizeFlag == 0 {
			sequence(treeID, seq, *batchSizeFlag)
		}
	}
	glog.Info("Finished queueing")
}

func traverseTreeStorage(ls storage.LogStorage, treeID int64, ts int, rev int64) {
	nodesAtLevel := int64(ts)

	tx, err := ls.SnapshotForTree(context.TODO(), treeID)
	if err != nil {
		glog.Fatalf("SnapshotForTree: %v", err)
	}
	defer func() {
		if err := tx.Commit(); err != nil {
			glog.Fatalf("TX Commit(): %v", err)
		}
	}()

	levels := int64(0)
	n := nodesAtLevel
	for n > 0 {
		levels++
		n = n >> 1
	}

	// Because of the way we store subtrees omitting internal RHS nodes with one sibling there
	// is an extra level stored for trees that don't have a number of leaves that is a power
	// of 2. We account for this here and in the loop below.
	if !isPerfectTree(int64(ts)) {
		levels++
	}

	for level := int64(0); level < levels; level++ {
		for node := int64(0); node < nodesAtLevel; node++ {
			// We're going to request one node at a time, which would normally be slow but we have
			// the tree in RAM so it's not a real problem.
			nodeID, err := storage.NewNodeIDForTreeCoords(level, node, 64)
			if err != nil {
				glog.Fatalf("NewNodeIDForTreeCoords: (%d, %d): got: %v, want: nil", level, node, err)
			}

			nodes, err := tx.GetMerkleNodes(context.TODO(), rev, []storage.NodeID{nodeID})
			if err != nil {
				glog.Fatalf("GetMerkleNodes: %s: %v", nodeID.CoordString(), err)
			}
			if len(nodes) != 1 {
				glog.Fatalf("GetMerkleNodes: %s: want 1 node got: %v", nodeID.CoordString(), nodes)
			}

			fmt.Printf("%6d %6d -> %s\n", level, node, hex.EncodeToString(nodes[0].Hash))
		}

		nodesAtLevel = nodesAtLevel >> 1
		fmt.Println()
		// This handles the extra level in non-perfect trees
		if nodesAtLevel == 0 {
			nodesAtLevel = 1
		}
	}
}

func dumpLeaves(ls storage.LogStorage, treeID int64, ts int) {
	tx, err := ls.SnapshotForTree(context.TODO(), treeID)
	if err != nil {
		glog.Fatalf("SnapshotForTree: %v", err)
	}
	defer func() {
		if err := tx.Commit(); err != nil {
			glog.Fatalf("TX Commit(): got: %v", err)
		}
	}()

	for l := int64(0); l < int64(ts); l++ {
		leaves, err := tx.GetLeavesByIndex(context.TODO(), []int64{l})
		if err != nil {
			glog.Fatalf("GetLeavesByIndex for index %d got: %v", l, err)
		}
		fmt.Printf("%6d:%s\n", l, leaves[0].LeafValue)
	}
}

func hexMap(in map[string][]byte) map[string][]byte {
	m := make(map[string][]byte)

	for k, v := range in {
		unb64, err := base64.StdEncoding.DecodeString(k)
		if err != nil {
			glog.Fatalf("Could not decode key as base 64: %s got: %v", k, err)
		}
		m[hex.EncodeToString(unb64)] = v
	}

	return m
}

func hexKeys(s *storagepb.SubtreeProto) {
	s.Leaves = hexMap(s.Leaves)
	s.InternalNodes = hexMap(s.InternalNodes)
}

func isPerfectTree(x int64) bool {
	return x != 0 && (x&(x-1) == 0)
}

func recordIOHdr() {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, recordIOMagic)
	if err != nil {
		glog.Fatalf("binary.Write failed: %v", err)
	}
	fmt.Print(buf.String())
}
