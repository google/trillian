// Copyright 2017 Google LLC. All Rights Reserved.
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
	"flag"
	"fmt"

	"github.com/golang/glog"
)

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

func main() {
	flag.Parse()
	defer glog.Flush()

	fmt.Print(Main(Options{
		TreeSize:       *treeSizeFlag,
		BatchSize:      *batchSizeFlag,
		LeafFormat:     *leafDataFormatFlag,
		LatestRevision: *latestRevisionFlag,
		Summary:        *summaryFlag,
		HexKeys:        *hexKeysFlag,
		LeafHashes:     *leafHashesFlag,
		RecordIO:       *recordIOFlag,
		Rebuild:        *rebuildInternalFlag,
		Traverse:       *traverseFlag,
		DumpLeaves:     *dumpLeavesFlag,
	}))
}
