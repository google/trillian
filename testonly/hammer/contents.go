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

package hammer

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sort"
	"sync"

	"github.com/golang/glog"
	"github.com/google/trillian"
)

// mapContents is a complete copy of the map's contents at a particular revision.
type mapContents struct {
	data map[mapKey]string
	rev  int64
}
type mapKey [sha256.Size]byte

func (m *mapContents) empty() bool {
	if m == nil {
		return true
	}
	return len(m.data) == 0
}

// pickKey randomly selects a key that already exists in a given copy of the
// map's contents. Assumes that the copy is non-empty.
func (m *mapContents) pickKey(prng *rand.Rand) []byte {
	if m.empty() {
		panic("internal error: can't pick a key, map data is empty!")
	}

	choice := prng.Intn(len(m.data))
	// Need sorted keys for reproduceability.
	keys := make([]mapKey, 0)
	for k := range m.data {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) == -1
	})
	return keys[choice][:]
}

// checkContents compares information returned from the Map against a local copy
// of the map's contents.
func (m *mapContents) checkContents(leafInclusions []*trillian.MapLeafInclusion, extraSize uint) error {
	for _, inc := range leafInclusions {
		leaf := inc.Leaf
		var key mapKey
		copy(key[:], leaf.Index)
		value, ok := m.data[key]
		if ok {
			if string(leaf.LeafValue) != value {
				return fmt.Errorf("got leaf[%v].LeafValue=%q, want %q", key, leaf.LeafValue, value)
			}
			if want := extraDataForValue(leaf.LeafValue, extraSize); !bytes.Equal(leaf.ExtraData, want) {
				return fmt.Errorf("got leaf[%v].ExtraData=%q, want %q", key, leaf.ExtraData, want)
			}
		} else {
			if len(leaf.LeafValue) > 0 {
				return fmt.Errorf("got leaf[%v].LeafValue=%q, want not-present", key, leaf.LeafValue)
			}
		}
	}
	return nil
}

// updatedWith returns a new mapContents object that has been updated to include the
// given leaves and revision.  A nil receiver object is allowed.
func (m *mapContents) updatedWith(rev uint64, leaves []*trillian.MapLeaf) *mapContents {
	// Start from previous map contents
	result := mapContents{rev: int64(rev), data: make(map[mapKey]string)}
	if m != nil {
		for k, v := range m.data {
			result.data[k] = v
		}
	}
	// Update with given leaves
	for _, leaf := range leaves {
		var k mapKey
		copy(k[:], leaf.Index)
		if leaf.LeafValue != nil {
			result.data[k] = string(leaf.LeafValue)
		} else {
			delete(result.data, k)
		}
	}

	return &result
}

// How many copies of map contents to hold on to.
const copyCount = 10

type versionedMapContents struct {
	mu sync.RWMutex

	// contents holds copies of the map at different revisions,
	// from later to earlier (so [0] is the most recent).
	contents [copyCount]*mapContents
}

func (p *versionedMapContents) empty() bool {
	return p.lastCopy() == nil
}

// prevCopy returns the specified copy of the map's contents.
func (p *versionedMapContents) prevCopy(which int) *mapContents {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.contents[which]
}

// lastCopy returns the most recent copy of the map's contents.
func (p *versionedMapContents) lastCopy() *mapContents {
	return p.prevCopy(0)
}

// pickCopy returns a previous copy of the map's contents, returning
// nil if there are no local copies.
func (p *versionedMapContents) pickCopy(prng *rand.Rand) *mapContents {
	p.mu.RLock()
	defer p.mu.RUnlock()
	// Count the number of filled copies.
	i := 0
	for ; i < copyCount; i++ {
		if p.contents[i] == nil {
			break
		}
	}
	if i == 0 {
		// No copied contents yet
		return nil
	}
	choice := prng.Intn(i)
	return p.contents[choice]
}

func (p *versionedMapContents) updateContentsWith(rev uint64, leaves []*trillian.MapLeaf) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Sanity check on rev being +ve and monotone increasing.
	if rev < 1 {
		return errInvariant{fmt.Sprintf("got rev %d, want >=1 when trying to update hammer state with contents", rev)}
	}
	if p.contents[0] != nil && int64(rev) <= p.contents[0].rev {
		return errInvariant{fmt.Sprintf("got rev %d, want >%d when trying to update hammer state with new contents", rev, p.contents[0].rev)}
	}

	// Shuffle earlier contents along.
	for i := copyCount - 1; i > 0; i-- {
		p.contents[i] = p.contents[i-1]
	}
	p.contents[0] = p.contents[1].updatedWith(rev, leaves)

	if glog.V(3) {
		p.dumpLockedContents()
	}
	return nil
}

// dumpLockedContents shows the local copies of the map's contents; it should be called with p.mu held.
func (p *versionedMapContents) dumpLockedContents() {
	fmt.Println("Contents\n~~~~~~~~")
	for i, c := range p.contents {
		if c == nil {
			break
		}
		fmt.Printf(" slot #%d\n", i)
		fmt.Printf("  revision: %d\n", c.rev)
		fmt.Println("  data:")
		for k, v := range c.data {
			fmt.Printf("   k: %s v: %v\n", string(k[:]), v)
		}
	}
	fmt.Println("~~~~~~~~")
}
