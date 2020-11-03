// Copyright 2020 Google LLC. All Rights Reserved.
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

// mapdemo is a simple example that shows how a verifiable map can be
// constructed in Beam.
package main

import (
	"context"
	"crypto"
	"flag"
	"fmt"
	"log"
	"reflect"

	"github.com/apache/beam/sdks/go/pkg/beam"
	"github.com/apache/beam/sdks/go/pkg/beam/io/textio"
	"github.com/apache/beam/sdks/go/pkg/beam/x/beamx"

	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/experimental/batchmap/tilepb"
	"github.com/google/trillian/merkle/coniks"
)

const hash = crypto.SHA512_256

var (
	output       = flag.String("output", "./tilehashes.txt", "Output file in which the summaries of the tiles will be written.")
	valueSalt    = flag.String("value_salt", "v1", "Some string that will be smooshed in with the generated value before hashing. Allows generated values to be deterministic but variable.")
	startKey     = flag.Int64("start_key", 0, "Keys will be generated starting with this index.")
	keyCount     = flag.Int64("key_count", 1<<10, "The number of keys that will be placed in the map.")
	treeID       = flag.Int64("tree_id", 12345, "The ID of the tree. Used as a salt in hashing.")
	prefixStrata = flag.Int("prefix_strata", 3, "The number of strata of 8-bit strata before the final strata. 3 is optimal for trees up to 2^30. 10 is required to import into Trillian.")
)

func init() {
	beam.RegisterFunction(tileToTextFn)
	beam.RegisterType(reflect.TypeOf((*mapEntryFn)(nil)).Elem())
}

func main() {
	flag.Parse()
	beam.Init()

	if *output == "" {
		log.Fatal("No output provided")
	}

	p, s := beam.NewPipelineWithRoot()

	// Get the collection of key/values that are to be committed to by the map.
	// Here we generate them from scratch, but a real application would likely want to commit to
	// data read from some data source.
	entries := beam.ParDo(s, &mapEntryFn{*valueSalt, *treeID}, createRange(s, *startKey, *keyCount))

	// Create the map, which will be returned as a collection of Tiles.
	allTiles, err := batchmap.Create(s, entries, *treeID, hash, *prefixStrata)

	if err != nil {
		log.Fatalf("Failed to create pipeline: %v", err)
	}

	// For the purpose of a simple demo, summarize each tile into a single-line string.
	summaries := beam.ParDo(s, tileToTextFn, allTiles)
	// Write this collection of summaries to the output file.
	textio.Write(s, *output, summaries)

	// All of the above constructs the pipeline but doesn't run it. Now we run it.
	if err := beamx.Run(context.Background(), p); err != nil {
		log.Fatalf("Failed to execute job: %v", err)
	}
}

// mapEntryFn is a Beam ParDo function that generates a key/value from an int64 input.
// In a real application this would be replaced with a function that generated the Entry
// objects from some domain objects (e.g. for Certificate Transparency this might take
// a Certificate as input and generate an Entry that represents it).
type mapEntryFn struct {
	Salt   string
	TreeID int64
}

func (fn *mapEntryFn) ProcessElement(i int64) *tilepb.Entry {
	h := hash.New()
	h.Write([]byte(fmt.Sprintf("%d", i)))
	kbs := h.Sum(nil)

	data := []byte(fmt.Sprintf("[%s]%d", fn.Salt, i))

	return &tilepb.Entry{
		HashKey:   kbs,
		HashValue: coniks.Default.HashLeaf(fn.TreeID, kbs, data),
	}
}

// tileToTextFn is a Beam ParDo function that generates a summary from a tile.
// This is purely to have a simple output format for the demo and a real application
// would need to store the generated Tile in some way in order to generate proofs.
func tileToTextFn(t *tilepb.Tile) string {
	return fmt.Sprintf("%x: %x (%d)", t.Path, t.RootHash, len(t.Leaves))
}

// createRange simply generates a PCollection of int64 which is used to seed the demo
// pipeline.
func createRange(s beam.Scope, start, count int64) beam.PCollection {
	// TODO(mhutchinson): make this parallel
	values := make([]int64, count)
	for i := int64(0); i < count; i++ {
		values[i] = start + i
	}
	return beam.CreateList(s, values)
}
