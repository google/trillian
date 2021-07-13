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

package tile

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/google/trillian/merkle/rfc6962"
)

func tc(h, l, i uint64) Coords {
	return Coords{TileHeight: h, Level: l, Index: i}
}
func tc2(l, i uint64) Coords {
	return tc(2, l, i)
}
func tc3(l, i uint64) Coords {
	return tc(3, l, i)
}

func TestCoordsString(t *testing.T) {
	tests := []struct {
		in   Coords
		want string
	}{
		{in: tc2(0, 0), want: "tile2(0,0)"},
		{in: tc(4, 3, 1), want: "tile4(3,1)"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("string-tile%d(%d,%d)", test.in.TileHeight, test.in.Level, test.in.Index), func(t *testing.T) {
			got := test.in.String()
			if got != test.want {
				t.Errorf("%+v.String()=%v, want %v", test.in, got, test.want)
			}
		})
	}
}

func TestCoordsBoundaries(t *testing.T) {
	tests := []struct {
		in         Coords
		wantAbove  MerkleCoords
		wantTop    [2]MerkleCoords
		wantBottom []MerkleCoords
	}{
		{
			in:         tc2(0, 0),
			wantAbove:  mc(2, 0),
			wantTop:    [2]MerkleCoords{mc(1, 0), mc(1, 1)},
			wantBottom: []MerkleCoords{mc(0, 0), mc(0, 1), mc(0, 2), mc(0, 3)},
		},
		{
			in:         tc2(0, 1),
			wantAbove:  mc(2, 1),
			wantTop:    [2]MerkleCoords{mc(1, 2), mc(1, 3)},
			wantBottom: []MerkleCoords{mc(0, 4), mc(0, 5), mc(0, 6), mc(0, 7)},
		},
		{
			in:         tc2(0, 2),
			wantAbove:  mc(2, 2),
			wantTop:    [2]MerkleCoords{mc(1, 4), mc(1, 5)},
			wantBottom: []MerkleCoords{mc(0, 8), mc(0, 9), mc(0, 10), mc(0, 11)},
		},
		{
			in:         tc2(0, 3),
			wantAbove:  mc(2, 3),
			wantTop:    [2]MerkleCoords{mc(1, 6), mc(1, 7)},
			wantBottom: []MerkleCoords{mc(0, 12), mc(0, 13), mc(0, 14), mc(0, 15)},
		},
		{
			in:         tc2(0, 4),
			wantAbove:  mc(2, 4),
			wantTop:    [2]MerkleCoords{mc(1, 8), mc(1, 9)},
			wantBottom: []MerkleCoords{mc(0, 16), mc(0, 17), mc(0, 18), mc(0, 19)},
		},
		{
			in:         tc2(0, 5),
			wantAbove:  mc(2, 5),
			wantTop:    [2]MerkleCoords{mc(1, 10), mc(1, 11)},
			wantBottom: []MerkleCoords{mc(0, 20), mc(0, 21), mc(0, 22), mc(0, 23)},
		},
		{
			in:         tc2(1, 0),
			wantAbove:  mc(4, 0),
			wantTop:    [2]MerkleCoords{mc(3, 0), mc(3, 1)},
			wantBottom: []MerkleCoords{mc(2, 0), mc(2, 1), mc(2, 2), mc(2, 3)},
		},
		{
			in:         tc3(0, 0),
			wantAbove:  mc(3, 0),
			wantTop:    [2]MerkleCoords{mc(2, 0), mc(2, 1)},
			wantBottom: []MerkleCoords{mc(0, 0), mc(0, 1), mc(0, 2), mc(0, 3), mc(0, 4), mc(0, 5), mc(0, 6), mc(0, 7)},
		},
		{
			in:         tc3(0, 1),
			wantAbove:  mc(3, 1),
			wantTop:    [2]MerkleCoords{mc(2, 2), mc(2, 3)},
			wantBottom: []MerkleCoords{mc(0, 8), mc(0, 9), mc(0, 10), mc(0, 11), mc(0, 12), mc(0, 13), mc(0, 14), mc(0, 15)},
		},
		{
			in:         tc3(1, 1),
			wantAbove:  mc(6, 1),
			wantTop:    [2]MerkleCoords{mc(5, 2), mc(5, 3)},
			wantBottom: []MerkleCoords{mc(3, 8), mc(3, 9), mc(3, 10), mc(3, 11), mc(3, 12), mc(3, 13), mc(3, 14), mc(3, 15)},
		},
	}
	for _, test := range tests {
		t.Run(test.in.String(), func(t *testing.T) {
			gotAbove := test.in.Above()
			if gotAbove != test.wantAbove {
				t.Errorf("%+v.Above()=%v, want %v", test.in, gotAbove, test.wantAbove)
			}
			gotTop := test.in.Top()
			if !reflect.DeepEqual(gotTop, test.wantTop) {
				t.Errorf("%+v.Top()={%v,%v}, want {%v,%v}", test.in, gotTop[0], gotTop[1], test.wantTop[0], test.wantTop[1])
			}
			gotBottom := test.in.Bottom()
			if !reflect.DeepEqual(gotBottom, test.wantBottom) {
				t.Errorf("%+v.Bottom()=%v, want %v", test.in, gotBottom, test.wantBottom)
			}
		})
	}
}

func TestCoordsBoundaryInvariants(t *testing.T) {
	for i := 0; i < 5000; i++ {
		tileHeight := uint64(1 + rand.Int63n(10))
		tileWidth := uint64(1) << tileHeight
		cLevel := uint64(rand.Int63n(40))
		level := cLevel / tileHeight
		idx := uint64(rand.Int63n(300))
		in := tc(tileHeight, level, idx)

		above := in.Above()
		top := in.Top()
		bottom := in.Bottom()

		if in.Includes(above) {
			t.Errorf("%s.Includes(%v)=true for above node, want false", in, above)
		}
		if top[0].Level != (above.Level-1) || top[1].Level != (above.Level-1) {
			t.Errorf("%s.Above()=%v level mismatch with %s.Top()={%v,%v}", in, above, in, top[0], top[1])
		}
		if !in.Includes(top[0]) {
			t.Errorf("%s.Includes(%v)=false for top[0] node, want true", in, top[0])
		}
		if !in.Includes(top[1]) {
			t.Errorf("%s.Includes(%v)=false for top[1] node, want true", in, top[1])
		}
		if len(bottom) != int(tileWidth) {
			t.Errorf("len(%s.Bottom()=%v)=%d, want %d", in, bottom, len(bottom), tileWidth)
		}
		prevIdx := bottom[0].Index - 1
		for i, c := range bottom {
			if c.Index != prevIdx+1 {
				t.Errorf("%s.Bottom()[%d]=%v has non-consecutive index", in, i, c)
			}
			prevIdx = c.Index
			if got, want := c.Level, above.Level-tileHeight; got != want {
				t.Errorf("%s.Bottom()[%d].Height=%d, want %d", in, i, got, want)
			}
			if !in.Includes(c) {
				t.Errorf("%s.Includes(%v)=false for bottom[%d] node, want true", in, c, i)
			}
			if got := CoordsFor(c, tileHeight); !reflect.DeepEqual(got, in) {
				t.Errorf("CoordsFor(%v,%d)=%v, want %v", c, tileHeight, got, in)
			}
		}
	}
}

func TestCoordsIncludes(t *testing.T) {
	tests := []struct {
		tile Coords
		in   MerkleCoords
		want bool
	}{
		{tile: tc2(0, 0), in: mc(0, 0), want: true},
		{tile: tc2(0, 0), in: mc(3, 0), want: false},
		{tile: tc2(0, 0), in: mc(0, 10), want: false},
		{tile: tc2(0, 0), in: mc(0, 10), want: false},
		{tile: tc3(1, 1), in: mc(6, 1), want: false},
		{tile: tc3(1, 1), in: mc(5, 1), want: false},
		{tile: tc3(1, 1), in: mc(5, 2), want: true},
		{tile: tc3(1, 1), in: mc(5, 3), want: true},
		{tile: tc3(1, 1), in: mc(3, 9), want: true},
		{tile: tc(8, 4, 255), in: mc(32, 65280), want: true},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s-in-%s", test.in, test.tile), func(t *testing.T) {
			got := test.tile.Includes(test.in)
			if got != test.want {
				t.Errorf("%s.Includes(%s)=%v, want %v", test.tile, test.in, got, test.want)
			}
			if test.want {
				// For an included node, check that a tiling with the same tile height
				// gets back to the test tile.
				if got := CoordsFor(test.in, test.tile.TileHeight); !reflect.DeepEqual(got, test.tile) {
					t.Errorf("CoordsFor(%v,%d)=%v, want %v", test.in, test.tile.TileHeight, got, test.tile)
				}
			}
		})
	}
}

func h2(l, r []byte) []byte {
	return rfc6962.DefaultHasher.HashChildren(l, r)
}

func hashes(count int) [][]byte {
	result := make([][]byte, count)
	for i := 0; i < count; i++ {
		result[i] = make([]byte, sha256.Size)
		rand.Read(result[i])
	}
	return result
}

func partialTile(h, l, i uint64, w int) *Tile {
	return &Tile{
		Coords: Coords{
			TileHeight: h,
			Level:      l,
			Index:      i,
		},
		Hashes:       hashes(w),
		hashChildren: h2,
	}
}
func fullTile(h, l, i uint64) *Tile {
	return partialTile(h, l, i, 1<<h)
}
func tile2(l, i uint64) *Tile {
	return fullTile(2, l, i)
}
func tile3(l, i uint64) *Tile {
	return fullTile(3, l, i)
}
func tile4(l, i uint64) *Tile {
	return fullTile(4, l, i)
}

func TestNewTile(t *testing.T) {
	tests := []struct {
		coords  Coords
		hashes  [][]byte
		wantErr string
	}{
		{
			coords:  tc2(0, 0),
			wantErr: "no hashes provided",
		},
		{
			coords:  tc2(0, 0),
			hashes:  [][]byte{},
			wantErr: "no hashes provided",
		},
		{
			coords:  tc2(0, 0),
			hashes:  hashes(7),
			wantErr: "too many hashes",
		},
		{
			coords: tc2(0, 0),
			hashes: hashes(1),
		},
	}
	for _, test := range tests {
		got, err := NewTile(test.coords, test.hashes, h2)
		if err != nil {
			if len(test.wantErr) == 0 {
				t.Errorf("NewTile(%v, |%d|)=nil,%v; want _,nil", test.coords, len(test.hashes), err)
			} else if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("NewTile(%v, |%d|)=nil,%v; want _,err containing %q", test.coords, len(test.hashes), err, test.wantErr)
			}
			continue
		}
		if len(test.wantErr) > 0 {
			t.Errorf("NewTile(%v, |%d|)=%v,nil; want _,err containing %q", test.coords, len(test.hashes), got, test.wantErr)
		}
	}
}

func TestTileString(t *testing.T) {
	tests := []struct {
		in   *Tile
		want string
	}{
		{in: tile2(0, 0), want: "tile2(0,0)"},
		{in: tile3(3, 1), want: "tile3(3,1)"},
		{in: tile4(3, 1), want: "tile4(3,1)"},
		{in: partialTile(4, 3, 1, 5), want: "tile4(3,1)/5"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("string-tile%d(%d,%d)", test.in.TileHeight, test.in.Level, test.in.Index), func(t *testing.T) {
			got := test.in.String()
			if got != test.want {
				t.Errorf("%+v.String()=%v, want %v", test.in, got, test.want)
			}
		})
	}
}

func TestTileExtends(t *testing.T) {
	t200 := fullTile(2, 0, 0)
	t201 := fullTile(2, 0, 1)
	t210 := fullTile(2, 1, 0)
	other := fullTile(2, 0, 0)
	t300 := fullTile(3, 0, 0)
	t200some := fullTile(2, 0, 0)
	t200some.Hashes = t200.Hashes[:2]
	tests := []struct {
		base, extension *Tile
		wantErr         string
	}{
		{base: t200, extension: t200},
		{base: t200, extension: other, wantErr: "different hash"},
		{base: t200, extension: t300, wantErr: "different height"},
		{base: t200, extension: t201, wantErr: "different coords"},
		{base: t200, extension: t210, wantErr: "different coords"},
		{base: t200, extension: t200some, wantErr: "fewer hashes"},
		{base: t200some, extension: t200},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%v-extends-%v", test.extension, test.base), func(t *testing.T) {
			err := test.extension.Extends(test.base)
			if err != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("%v.Extends(%v)=%v, want nil", test.base, test.extension, err)
				} else if !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("%v.Extends(%v)=%v, want err containing %q", test.base, test.extension, err, test.wantErr)
				}
				return
			}
			if len(test.wantErr) > 0 {
				t.Errorf("%v.Extends(%v)=nil, want err containing %q", test.base, test.extension, test.wantErr)
			}
		})
	}
}

func TestTileRootHash(t *testing.T) {
	t200 := fullTile(2, 0, 0)
	rootHash := h2(h2(t200.Hashes[0], t200.Hashes[1]), h2(t200.Hashes[2], t200.Hashes[3]))
	t200fill2 := partialTile(2, 0, 0, 2)
	tests := []struct {
		tile *Tile
		want []byte // nil indicates incomplete tile
	}{
		{tile: t200fill2},
		{tile: t200, want: rootHash},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("tile-%s", test.tile), func(t *testing.T) {
			if gotComplete, wantComplete := test.tile.IsComplete(), len(test.want) > 0; gotComplete != wantComplete {
				t.Errorf("%v.IsComplete()=%v, want %v", test.tile, gotComplete, wantComplete)
			}
			got, err := test.tile.RootHash()
			if err != nil {
				if len(test.want) > 0 {
					t.Errorf("%v.RootHash()=nil,%v, want %x,nil", test.tile, err, test.want)
				}
				return
			}
			if !bytes.Equal(got, test.want) {
				t.Errorf("%v.RootHash()=%x, want %x", test.tile, got, test.want)
			}
		})
	}
}

func TestTileHashFor(t *testing.T) {
	t200 := fullTile(2, 0, 0)
	t200fill2 := partialTile(2, 0, 0, 2)
	t301fill7 := partialTile(3, 0, 1, 7)
	tests := []struct {
		tile    *Tile
		c       MerkleCoords
		want    []byte
		wantErr string
	}{
		{
			tile: t200,
			c:    mc(0, 0),
			want: t200.Hashes[0],
		},
		{
			tile: t200,
			c:    mc(0, 3),
			want: t200.Hashes[3],
		},
		{
			tile: t200,
			c:    mc(1, 0),
			want: h2(t200.Hashes[0], t200.Hashes[1]),
		},
		{
			tile:    t200,
			c:       mc(1, -3),
			wantErr: "pseudo-node",
		},
		{
			tile:    t200,
			c:       mc(0, 4),
			wantErr: "not included in tile",
		},
		{
			tile:    t200,
			c:       mc(3, 0),
			wantErr: "not included in tile",
		},
		{
			tile:    t200fill2,
			c:       mc(0, 3),
			wantErr: "not available in incomplete tile",
		},
		{
			tile: t200fill2,
			c:    mc(1, 0),
			want: h2(t200fill2.Hashes[0], t200fill2.Hashes[1]),
		},
		{
			tile:    t200fill2,
			c:       mc(1, 1),
			wantErr: "not available in incomplete tile",
		},
		{
			tile: t301fill7,
			c:    mc(2, 2),
			want: h2(h2(t301fill7.Hashes[0], t301fill7.Hashes[1]), h2(t301fill7.Hashes[2], t301fill7.Hashes[3])),
		},
		{
			tile:    t301fill7,
			c:       mc(2, 3),
			wantErr: "not available in incomplete tile",
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("hash-%s-in-%s", test.c, test.tile), func(t *testing.T) {
			got, err := test.tile.HashFor(test.c)
			if err != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("%v.HashFor(%v)=nil,%v; want _,nil", test.tile, test.c, err)
				} else if !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("%v.HashFor(%v)=nil,%v; want nil,err containing %q", test.tile, test.c, err, test.wantErr)
				}
				return
			}
			if len(test.wantErr) != 0 {
				t.Fatalf("%s.HashFor(%v)=%x, nil; want nil,err containing %q", test.tile, test.c, got, test.wantErr)
			}
			if !bytes.Equal(got, test.want) {
				t.Errorf("%s.HashFor(%v)=%x; want %x", test.tile, test.c, got, test.want)
			}
		})
	}
}
