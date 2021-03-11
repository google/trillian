// Copyright 2016 Google LLC. All Rights Reserved.
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

package testonly

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian/storage/tree"
)

type nodeIDEq struct {
	expectedID tree.NodeID
}

// Matches implements the gomock.Matcher API.
func (m nodeIDEq) Matches(x interface{}) bool {
	n, ok := x.(tree.NodeID)
	if !ok {
		return false
	}
	return n.Equivalent(m.expectedID)
}

func (m nodeIDEq) String() string {
	return fmt.Sprintf("is %s", m.expectedID.String())
}

// NodeIDEq returns a matcher that expects the specified NodeID.
func NodeIDEq(n tree.NodeID) gomock.Matcher {
	return nodeIDEq{n}
}

// NodeSet returns a matcher that expects the given set of nodes.
func NodeSet(nodes []tree.Node) gomock.Matcher {
	return nodeSet(sorted(nodes))
}

type nodeSet []tree.Node

// Matches implements the gomock.Matcher API.
func (n nodeSet) Matches(x interface{}) bool {
	nodes, ok := x.([]tree.Node)
	if !ok {
		return false
	}
	return reflect.DeepEqual(sorted(nodes), []tree.Node(n))
}

func (n nodeSet) String() string {
	return fmt.Sprintf("equivalent to %v", []tree.Node(n))
}

func sorted(n []tree.Node) []tree.Node {
	sort.Slice(n, func(i, j int) bool {
		return n[i].ID.Level < n[j].ID.Level ||
			(n[i].ID.Level == n[j].ID.Level && n[i].ID.Index < n[j].ID.Index)
	})
	return n
}
