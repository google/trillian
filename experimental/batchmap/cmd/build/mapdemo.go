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
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/io/filesystem/local"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/x/beamx"
	"k8s.io/klog/v2"

	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/smt/node"
)

const hash = crypto.SHA512_256

var (
	output       = flag.String("output", "", "Output directory in which the tiles will be written.")
	valueSalt    = flag.String("value_salt", "v1", "Some string that will be smooshed in with the generated value before hashing. Allows generated values to be deterministic but variable.")
	startKey     = flag.Int64("start_key", 0, "Keys will be generated starting with this index.")
	keyCount     = flag.Int64("key_count", 1<<5, "The number of keys that will be placed in the map.")
	treeID       = flag.Int64("tree_id", 12345, "The ID of the tree. Used as a salt in hashing.")
	prefixStrata = flag.Int("prefix_strata", 1, "The number of strata of 8-bit strata before the final strata. 3 is optimal for trees up to 2^30. 10 is required to import into Trillian.")
)

func init() {
	beam.RegisterType(reflect.TypeOf((*mapEntryFn)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*writeTileFn)(nil)).Elem())
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	beam.Init()

	output := filepath.Clean(*output)
	if output == "" {
		klog.Exitf("No output provided")
	}

	// Create the directory if it doesn't exist
	if _, err := os.Stat(output); os.IsNotExist(err) {
		if err = os.Mkdir(output, 0o700); err != nil {
			klog.Fatalf("couldn't find or create directory %s, %v", output, err)
		}
	}

	p, s := beam.NewPipelineWithRoot()

	// Get the collection of key/values that are to be committed to by the map.
	// Here we generate them from scratch, but a real application would likely want to commit to
	// data read from some data source.
	entries := beam.ParDo(s, &mapEntryFn{*valueSalt, *treeID}, createRange(s, *startKey, *keyCount))

	// Create the map, which will be returned as a collection of Tiles.
	allTiles, err := batchmap.Create(s, entries, *treeID, hash, *prefixStrata)
	if err != nil {
		klog.Fatalf("Failed to create pipeline: %v", err)
	}

	// Write this collection of tiles to the output directory.
	beam.ParDo0(s, &writeTileFn{output}, allTiles)

	// All of the above constructs the pipeline but doesn't run it. Now we run it.
	if err := beamx.Run(context.Background(), p); err != nil {
		klog.Fatalf("Failed to execute job: %v", err)
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

func (fn *mapEntryFn) ProcessElement(i int64) *batchmap.Entry {
	h := hash.New()
	h.Write([]byte(fmt.Sprintf("%d", i)))
	kbs := h.Sum(nil)
	leafID := node.NewID(string(kbs), uint(len(kbs)*8))

	data := []byte(fmt.Sprintf("[%s]%d", fn.Salt, i))

	return &batchmap.Entry{
		HashKey:   kbs,
		HashValue: coniks.Default.HashLeaf(fn.TreeID, leafID, data),
	}
}

// writeTileFn serializes the tile into the given directory, using the tile
// path to determine the file name.
// This is reasonable for a demo with a small number of tiles, but with large
// maps with multiple revisions, it is conceivable that one could run out of
// inodes on the filesystem, and thus using a database locally or storing the
// tile data in cloud storage are more likely to scale.
type writeTileFn struct {
	Directory string
}

func (fn *writeTileFn) ProcessElement(ctx context.Context, t *batchmap.Tile) error {
	fs := local.New(ctx)
	w, err := fs.OpenWrite(ctx, fmt.Sprintf("%s/path_%x", fn.Directory, t.Path))
	if err != nil {
		return err
	}

	defer func() {
		if err := w.Close(); err != nil {
			klog.Errorf("Close(): %v", err)
		}
	}()

	bs, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = w.Write(bs)
	return err
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
