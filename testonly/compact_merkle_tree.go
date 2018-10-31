// Copyright 2016 Google Inc. All Rights Reserved.
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

// MerkleTreeLeafTestInputs returns a slice of leaf inputs that may be used in
// compact Merkle tree test cases.  They are intended to be added successively,
// so that after each addition the corresponding root from MerkleTreeLeafTestRoots
// gives the expected Merkle tree root hash.
func MerkleTreeLeafTestInputs() [][]byte {
	return [][]byte{
		[]byte(""), []byte("\x00"), []byte("\x10"), []byte("\x20\x21"), []byte("\x30\x31"),
		[]byte("\x40\x41\x42\x43"), []byte("\x50\x51\x52\x53\x54\x55\x56\x57"),
		[]byte("\x60\x61\x62\x63\x64\x65\x66\x67\x68\x69\x6a\x6b\x6c\x6d\x6e\x6f")}
}

// MerkleTreeLeafTestRootHashes returns a slice of Merkle tree root hashes that
// correspond to the expected tree state for the leaf additions returned by
// MerkleTreeLeafTestInputs(), as described above.
func MerkleTreeLeafTestRootHashes() [][]byte {
	return [][]byte{
		// constants from C++ test: https://github.com/google/certificate-transparency/blob/master/cpp/merkletree/merkle_tree_test.cc#L277
		MustHexDecode("6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"),
		MustHexDecode("fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125"),
		MustHexDecode("aeb6bcfe274b70a14fb067a5e5578264db0fa9b51af5e0ba159158f329e06e77"),
		MustHexDecode("d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7"),
		MustHexDecode("4e3bbb1f7b478dcfe71fb631631519a3bca12c9aefca1612bfce4c13a86264d4"),
		MustHexDecode("76e67dadbcdf1e10e1b74ddc608abd2f98dfb16fbce75277b5232a127f2087ef"),
		MustHexDecode("ddb89be403809e325750d3d263cd78929c2942b7942a34b77e122c9594a74c8c"),
		MustHexDecode("5dc9da79a70659a9ad559cb701ded9a2ab9d823aad2f4960cfe370eff4604328")}
}

// CompactMerkleTreeLeafTestNodeHashes returns the compact Tree.node state
// that must result after each of the leaf additions returned by
// MerkleTreeLeafTestInputs(), as described above.
func CompactMerkleTreeLeafTestNodeHashes() [][][]byte {
	return [][][]byte{
		nil, // perfect tree size, 2^0
		nil, // perfect tree size, 2^1
		{MustDecodeBase64("ApjRIpBtz8EIkstTpzmS/FufST6kybrbJ7eRtBJ6f+c="), MustDecodeBase64("+sVCA+fMaWzw38tCySodnbr3CtnmIfS9jZhmLwDjwSU=")},
		nil, // perfect tree size, 2^2
		{MustDecodeBase64("vBoGQ7EuTS18d5GPROD095qDi2z57FtcKD4fTYhZnms="), nil, MustDecodeBase64("037kGJdt2VdTwcc4Yrk5j6Kiz5tP8P3+izDNlSCWFLc=")},
		{nil, MustDecodeBase64("DrxdNDf74tsVi58Sah0RjjCBgQMdCpSfje3t68VY72o="), MustDecodeBase64("037kGJdt2VdTwcc4Yrk5j6Kiz5tP8P3+izDNlSCWFLc=")},
		{MustDecodeBase64("sIaT7C5yFZcTBkHoIR5+7cy0wmQTlj7ubB4u0W/7Gl8="), MustDecodeBase64("DrxdNDf74tsVi58Sah0RjjCBgQMdCpSfje3t68VY72o="), MustDecodeBase64("037kGJdt2VdTwcc4Yrk5j6Kiz5tP8P3+izDNlSCWFLc=")},
		nil, // perfect tree size, 2^3
	}
}

// EmptyMerkleTreeRootHash returns the expected root hash for an empty Merkle Tree
// that uses SHA256 hashing.
func EmptyMerkleTreeRootHash() []byte {
	const sha256EmptyTreeHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	return MustHexDecode(sha256EmptyTreeHash)
}
