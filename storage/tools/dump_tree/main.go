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
// Print the latest revision of each subtree proto for a tree of size 127 with hex keys:
// dump_tree -tree_size 127
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
	rebuildInternalFlag = flag.Bool("rebuild", true, "If true rebuilds internal nodes + root hash from leaves")
)

func main() {
	flag.Parse()
	defer glog.Flush()

	fmt.Print(Main(Options{
		TreeSize:   *treeSizeFlag,
		BatchSize:  *batchSizeFlag,
		LeafFormat: *leafDataFormatFlag,
		Rebuild:    *rebuildInternalFlag,
	}))
}
