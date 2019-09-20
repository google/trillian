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

package tree

// TileID holds the ID of a tile, which is aligned with the tree layout.
//
// It assumes that strata heights are multiples of 8, and so the byte
// representation of the TileID matches NodeID.
type TileID struct {
	Root NodeID
}

// AsKey returns the ID as a string suitable for in-memory mapping.
func (t TileID) AsKey() string {
	return string(t.AsBytes())
}

// AsBytes returns the ID as a byte slice suitable for passing in to the
// storage layer. The returned bytes must not be modified.
func (t TileID) AsBytes() []byte {
	// TODO(pavelkalinnikov): We could simply return t.Root.Path, but some NodeID
	// constructors allocate more bytes in Path than necessary.
	return t.Root.Path[:t.Root.PrefixLenBits/8]
}
