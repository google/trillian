// Copyright 2018 Google Inc. All Rights Reserved.
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

package merkle

import (
	"bytes"
	"crypto"
	"testing"

	"github.com/google/trillian/merkle/rfc6962"
)

var (
	leaf1 = []byte("this is leaf data 1")
	leaf2 = []byte("this is leaf data 2")
	leaf3 = []byte("this is leaf data 3")
)

// Test that chainInner does not mutate it's input slice and computes hashes correctly.
// Compare against values produced by rfc6962.DefaultHasher.
func TestChainInner(t *testing.T) {
	refHash := rfc6962.DefaultHasher

	type args struct {
		seed  []byte
		proof [][]byte
		index int64
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "one even",
			args: args{
				seed:  leaf1,
				index: 0,
				proof: [][]byte{leaf2},
			},
			want: refHash.HashChildren(leaf1, leaf2),
		},
		{
			name: "one odd",
			args: args{
				seed:  leaf1,
				index: 1,
				proof: [][]byte{leaf2},
			},
			want: refHash.HashChildren(leaf2, leaf1),
		},
		{
			name: "multiple odd",
			args: args{
				seed:  leaf1,
				index: 3,
				proof: [][]byte{leaf2, leaf3},
			},
			want: refHash.HashChildren(leaf3, refHash.HashChildren(leaf2, leaf1)),
		},
		{
			name: "multiple even",
			args: args{
				seed:  leaf1,
				index: 0,
				proof: [][]byte{leaf2, leaf3},
			},
			want: refHash.HashChildren(refHash.HashChildren(leaf1, leaf2), leaf3),
		},
		{
			name: "mixed",
			args: args{
				seed:  leaf1,
				index: 2,
				proof: [][]byte{leaf2, leaf3},
			},
			want: refHash.HashChildren(leaf3, refHash.HashChildren(leaf1, leaf2)),
		},
	}
	for _, tt := range tests {
		inSeed := append(make([]byte, 0, len(tt.args.seed)), tt.args.seed...)
		t.Run(tt.name, func(t *testing.T) {
			c := hashChainer{
				hasher: rfc6962.NewInplace(crypto.SHA256),
			}
			if got := c.chainInner(inSeed, tt.args.proof, tt.args.index); !bytes.Equal(got, tt.want) {
				t.Errorf("hashChainer.chainInner(%s) = %v, want %v", tt.name, got, tt.want)
			}
			if got := bytes.Equal(inSeed, tt.args.seed); got != true {
				t.Errorf("hashChainer.chainInner(%s): input slice was mutated", tt.name)
			}
		})
	}
}

// Test that chainInnerRight does not mutate it's input slice and computes hashes correctly.
func TestChainInnerRight(t *testing.T) {
	refHash := rfc6962.DefaultHasher

	type args struct {
		seed  []byte
		proof [][]byte
		index int64
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "no hashing",
			args: args{
				seed:  leaf1,
				index: 0,
				proof: [][]byte{leaf2},
			},
			want: leaf1,
		},
		{
			name: "hash once",
			args: args{
				seed:  leaf1,
				index: 1,
				proof: [][]byte{leaf2},
			},
			want: refHash.HashChildren(leaf2, leaf1),
		},
		{
			name: "hash once of two",
			args: args{
				seed:  leaf1,
				index: 1,
				proof: [][]byte{leaf2, leaf3},
			},
			want: refHash.HashChildren(leaf2, leaf1),
		},
		{
			name: "hash twice of two",
			args: args{
				seed:  leaf1,
				index: 3,
				proof: [][]byte{leaf2, leaf3},
			},
			want: refHash.HashChildren(leaf3, refHash.HashChildren(leaf2, leaf1)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inSeed := append(make([]byte, 0, len(tt.args.seed)), tt.args.seed...)
			c := hashChainer{
				hasher: rfc6962.NewInplace(crypto.SHA256),
			}
			if got := c.chainInnerRight(inSeed, tt.args.proof, tt.args.index); !bytes.Equal(got, tt.want) {
				t.Errorf("hashChainer.chainInnerRight() = %v, want %v", got, tt.want)
			}
			if got := bytes.Equal(inSeed, tt.args.seed); got != true {
				t.Errorf("hashChainer.chainInnerRight(%s): input slice was mutated", tt.name)
			}
		})
	}
}

// chainBorderRight is allowed to mutate the input slice.
func TestChainBorderRight(t *testing.T) {
	refHash := rfc6962.DefaultHasher

	type args struct {
		seed  []byte
		proof [][]byte
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "hash once",
			args: args{
				seed:  leaf1,
				proof: [][]byte{leaf2},
			},
			want: refHash.HashChildren(leaf2, leaf1),
		},
		{
			name: "hash twice",
			args: args{
				seed:  leaf1,
				proof: [][]byte{leaf2, leaf3},
			},
			want: refHash.HashChildren(leaf3, refHash.HashChildren(leaf2, leaf1)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := hashChainer{
				hasher: rfc6962.NewInplace(crypto.SHA256),
			}
			if got := c.chainBorderRight(tt.args.seed, tt.args.proof); !bytes.Equal(got, tt.want) {
				t.Errorf("hashChainer.chainBorderRight() = %v, want %v", got, tt.want)
			}
		})
	}
}
