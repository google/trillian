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

package storagepb

import (
	"fmt"

	"github.com/google/trillian/merkle/smt"
	"github.com/google/trillian/storage/tree"
)

// Unmarshal converts the given SubtreeProto to a Merkle tree tile.
func Unmarshal(sp *SubtreeProto) (smt.Tile, error) {
	if d := sp.GetDepth(); d <= 0 {
		return smt.Tile{}, fmt.Errorf("wrong depth %d, want > 0", d)
	}
	height := uint(sp.GetDepth())
	prefix := string(sp.GetPrefix())
	id := tree.NewNodeID2(prefix, uint(len(prefix)*8))
	if len(sp.GetLeaves()) == 0 {
		return smt.Tile{ID: id}, nil
	}

	tailBits := uint8((height-1)%8 + 1) // Bits of the last byte to use.
	leaves := make([]smt.NodeUpdate, 0, len(sp.GetLeaves()))
	for idStr, hash := range sp.GetLeaves() {
		suf, err := tree.ParseSuffix(idStr) // Note: No allocation if height <= 8.
		if err != nil {
			return smt.Tile{}, fmt.Errorf("%s: ParseSuffix: %v", idStr, err)
		} else if bits := uint(suf.Bits()); bits != height {
			return smt.Tile{}, fmt.Errorf("%s: wrong suffix bits %d, want %d", idStr, bits, height)
		}
		path := suf.Path() // TODO(pavelkalinnikov): Avoid the copying here.
		count := len(path)
		bytes := prefix + string(path[:count-1]) // Note: No allocation if height <= 8.
		id := tree.NewNodeID2WithLast(bytes, path[count-1], tailBits)
		leaves = append(leaves, smt.NodeUpdate{ID: id, Hash: hash})
	}
	// Canonicalize the leaves list.
	if err := smt.Prepare(leaves, id.BitLen()+height); err != nil {
		return smt.Tile{}, fmt.Errorf("Prepare: %v", err)
	}
	return smt.Tile{ID: id, Leaves: leaves}, nil
}

// Marshal converts the given Merkle tree tile to SubtreeProto.
func Marshal(t smt.Tile, height uint) (*SubtreeProto, error) {
	if height == 0 || height > 255 {
		return nil, fmt.Errorf("height out of [1,255] range: %d", height)
	}
	prefBits := t.ID.BitLen()
	if prefBits%8 != 0 {
		return nil, fmt.Errorf("tile root unaligned: %d", prefBits)
	}
	prefBytes := prefBits / 8
	bits := prefBits + height

	leaves := make(map[string][]byte, len(t.Leaves))
	for _, upd := range t.Leaves {
		id := upd.ID
		// TODO(pavelkalinnikov): These must move to be part of Tile contract.
		if bl := id.BitLen(); bl != bits {
			return nil, fmt.Errorf("wrong ID bits %d, want %d", bl, bits)
		}
		if id.Prefix(prefBits) != t.ID {
			return nil, fmt.Errorf("unrelated leaf ID: %v", id)
		}
		last, _ := id.LastByte()
		// TODO(pavelkalinnikov): Avoid allocation for height <= 8.
		path := []byte(id.FullBytes()[prefBytes:] + string([]byte{last}))
		suf := tree.NewSuffix(uint8(height), path)
		leaves[suf.String()] = upd.Hash
	}
	id := tree.NewNodeIDFromID2(t.ID)
	return &SubtreeProto{Prefix: id.Path, Depth: int32(height), Leaves: leaves}, nil
}
