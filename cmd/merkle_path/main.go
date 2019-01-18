// Copyright 2019 Google Inc. All Rights Reserved.
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

package main

import (
	"encoding/hex"
	"flag"
	"fmt"

	"github.com/golang/glog"
	_ "github.com/golang/protobuf/proto"
	_ "github.com/google/trillian/crypto/keyspb"
	_ "github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/merkle"
	_ "github.com/google/trillian/storage/storagepb"
)

var (
	size1    = flag.Int64("size1", 0, "First tree size (or only for inclusion proof)")
	size2    = flag.Int64("size2", 0, "Second tree size (for consistency proof)")
	index    = flag.Int64("index", 0, "Desired leaf index (for inclusion proof)")
	treeSize = flag.Int64("tree_size", 0, "Size of the Merkle tree")
)

func main() {
	flag.Parse()
	defer glog.Flush()

	var err error
	var path []merkle.NodeFetch
	subtrees := make(map[string]bool)

	if *size2 == 0 {
		// Do an inclusion proof
		fmt.Printf("Inclusion proof for index %d at tree size %d in a tree of size %d\n\n", *index, *size1, *treeSize)
		path, err = merkle.CalcInclusionProofNodeAddresses(*size1, *index, *treeSize, 64)
	} else {
		// It's a consistency proof.
		fmt.Printf("Consistency proof for tree size %d -> %d in a tree of size %d\n\n", *size1, *size2, *treeSize)
		path, err = merkle.CalcConsistencyProofNodeAddresses(*size1, *size2, *treeSize, 64)
	}

	if err != nil {
		glog.Exitf("Failed to build the Merkle path: %v", err)
	}

	fmt.Printf("Resulting path length: %d\n", len(path))
	for _, fetch := range path {
		nodeID := fetch.NodeID
		fmt.Printf("%v %v\n", nodeID.CoordString(), fetch.Rehash)
		prefixBytes := nodeID.PrefixLenBits >> 3
		// Might have to split the path if it isn't the root of a subtree.
		suffixBits := nodeID.PrefixLenBits - (prefixBytes << 3)
		subtree := nodeID.Path
		if suffixBits > 0 {
			subtree, _ = nodeID.Split(prefixBytes, suffixBits)
		}
		subtrees[hex.EncodeToString(subtree)] = true
	}

	fmt.Printf("\nPath traverses %d subtree(s)\n\n", len(subtrees))
	for subtree := range subtrees {
		fmt.Printf("%s %d\n", subtree, len(subtree)/2)
	}
}
