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

package core

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/merkle/testonly"
	"github.com/google/trillian/skylog/storage"
)

// TODO(pavelkalinnikov): Use GoMock?
type mockTreeWriter struct {
	calls [][]storage.Node
}

func (t *mockTreeWriter) Write(ctx context.Context, nodes []storage.Node) error {
	t.calls = append(t.calls, nodes)
	return nil
}

func (t *mockTreeWriter) verify(nodes []storage.Node) error {
	if len(t.calls) == 0 && len(nodes) == 0 {
		return nil
	}
	if got, want := len(t.calls), 1; got != want {
		return fmt.Errorf("wrong number of calls %d, want %d", got, want)
	}
	if got, want := t.calls[0], nodes; !reflect.DeepEqual(got, want) {
		return fmt.Errorf("got nodes: %+v\nwant: %+v", got, want)
	}
	return nil
}

func TestBuildWorker(t *testing.T) {
	hashes := testonly.NodeHashes()
	n := len(hashes[0])
	if got, want := n, 8; got != want {
		t.Fatalf("got %d leaves, want %d", got, want)
	}
	node := func(level uint, index uint64) storage.Node {
		return storage.Node{ID: compact.NewNodeID(level, index), Hash: hashes[level][index]}
	}
	adds := [][]storage.Node{
		{node(0, 0)},
		{node(0, 1), node(1, 0)},
		{node(0, 2)},
		{node(0, 3), node(1, 1), node(2, 0)},
		{node(0, 4)},
		{node(0, 5), node(1, 2)},
		{node(0, 6)},
		{node(0, 7), node(1, 3), node(2, 1), node(3, 0)},
	}

	rf := &compact.RangeFactory{Hash: rfc6962.DefaultHasher.HashChildren}
	var wantAdds []storage.Node
	for end := 0; end <= n; end++ {
		// TODO(pavelkalinnikov): Add tests where Begin > 0.
		t.Run(fmt.Sprintf("0:%d", end), func(t *testing.T) {
			var tw mockTreeWriter
			bw := NewBuildWorker(&tw, rf)
			bj := BuildJob{Begin: 0, Hashes: hashes[0][0:end]}
			if _, err := bw.Process(context.Background(), bj); err != nil {
				t.Fatalf("Process: %v", err)
			}
			if err := tw.verify(wantAdds); err != nil {
				t.Errorf("verify: %v", err)
			}
		})
		if end < n {
			wantAdds = append(wantAdds, adds[end]...)
		}
	}
}
