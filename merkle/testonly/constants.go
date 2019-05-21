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

// Package testonly contains code and data for testing Merkle trees.
package testonly

import "github.com/google/trillian/testonly"

var hex = testonly.MustHexDecode

// LeafInputs returns a slice of leaf inputs for testing Merkle trees.
func LeafInputs() [][]byte {
	return [][]byte{
		hex(""),
		hex("00"),
		hex("10"),
		hex("2021"),
		hex("3031"),
		hex("40414243"),
		hex("5051525354555657"),
		hex("606162636465666768696a6b6c6d6e6f"),
	}
}

// NodeHashes returns a structured slice of node hashes for all complete
// subtrees of a Merkle tree built from LeafInputs() using the RFC 6962 hashing
// strategy. The first index in the slice is the tree level (zero being the
// leaves level), the second is the horizontal index within a level.
func NodeHashes() [][][]byte {
	return [][][]byte{{
		hex("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"),
		hex("96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7"),
		hex("0298d122906dcfc10892cb53a73992fc5b9f493ea4c9badb27b791b4127a7fe7"),
		hex("07506a85fd9dd2f120eb694f86011e5bb4662e5c415a62917033d4a9624487e7"),
		hex("bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b"),
		hex("4271a26be0d8a84f0bd54c8c302e7cb3a3b5d1fa6780a40bcce2873477dab658"),
		hex("b08693ec2e721597130641e8211e7eedccb4c26413963eee6c1e2ed16ffb1a5f"),
		hex("46f6ffadd3d06a09ff3c5860d2755c8b9819db7df44251788c7d8e3180de8eb1"),
	}, {
		hex("fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125"),
		hex("5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e"),
		hex("0ebc5d3437fbe2db158b9f126a1d118e308181031d0a949f8dededebc558ef6a"),
		hex("ca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0"),
	}, {
		hex("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7"),
		hex("6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4"),
	}, {
		hex("5dc9da79a70659a9ad559cb701ded9a2ab9d823aad2f4960cfe370eff4604328"),
	}}
}

// RootHashes returns a slice of Merkle tree root hashes for all subsequent
// trees built from LeafInputs() using the RFC 6962 hashing strategy. Hashes
// are indexed by tree size starting from an empty tree.
func RootHashes() [][]byte {
	return [][]byte{
		EmptyRootHash(),
		hex("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"),
		hex("fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125"),
		hex("aeb6bcfe274b70a14fb067a5e5578264db0fa9b51af5e0ba159158f329e06e77"),
		hex("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7"),
		hex("4e3bbb1f7b478dcfe71fb631631519a3bca12c9aefca1612bfce4c13a86264d4"),
		hex("76e67dadbcdf1e10e1b74ddc608abd2f98dfb16fbce75277b5232a127f2087ef"),
		hex("ddb89be403809e325750d3d263cd78929c2942b7942a34b77e122c9594a74c8c"),
		hex("5dc9da79a70659a9ad559cb701ded9a2ab9d823aad2f4960cfe370eff4604328"),
	}
}

// CompactTreesOld returns a slice of compact.Tree internal hashes, in the old
// format, for all subsequent trees built from LeafInputs() using the RFC 6962
// hashing strategy.
//
// TODO(pavelkalinnikov): Remove it.
func CompactTreesOld() [][][]byte {
	nh := NodeHashes()
	return [][][]byte{
		nil, // Empty tree.
		nil, // Perfect tree size, 2^0.
		nil, // Perfect tree size, 2^1.
		{nh[0][2], nh[1][0]},
		nil, // Perfect tree size, 2^2.
		{nh[0][4], nil, nh[2][0]},
		{nil, nh[1][2], nh[2][0]},
		{nh[0][6], nh[1][2], nh[2][0]},
		nil, // Perfect tree size, 2^3.
	}
}

// EmptyRootHash returns the root hash for an empty Merkle tree that uses
// SHA256-based strategy from RFC 6962.
func EmptyRootHash() []byte {
	return hex("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
}
