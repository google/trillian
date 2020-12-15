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

package batchmap

// Tile represents a perfect subtree covering the whole height of a stratum
// within a sparse map.
type Tile struct {
	// The path from the root of the map to the root of this tile.
	Path []byte
	// The computed hash of this subtree.
	// TODO(mhutchinson): Consider removing this.
	RootHash []byte
	// All non-empty leaves in this tile, sorted left-to-right.
	Leaves []*TileLeaf
}

// TileLeaf is a leaf value of a Tile.
// If it belongs to a leaf tile then this represents one of the values that the
// map commits to. Otherwise, this leaf represents the root of the subtree in
// the stratum below.
type TileLeaf struct {
	// The path from the root of the container Tile to this leaf.
	Path []byte
	// The hash value being committed to.
	Hash []byte
}

// Entry is a single key/value to be committed to by the map.
type Entry struct {
	// The key that uniquely identifies this key/value.
	// These keys must be distributed uniformly and randomly across the key space.
	HashKey []byte
	// Hash of the value to be committed to. This will literally be set as a hash
	// of a TileLeaf in a leaf Tile.
	// It is the job of the code constructing this Entry to ensure that this has
	// appropriate preimage protection and domain separation. This means this will
	// likely be set to something like H(salt || data).
	//
	// TODO(mhutchinson): Revisit this. My preference is that this is set to
	// H(data), and the map synthesis will set the hash to H(salt||H(data)).
	// This allows the map to always be constructed with good security.
	HashValue []byte
}
