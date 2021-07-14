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

package merkle

import (
	"github.com/google/trillian/merkle/proof"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CalcConsistencyProofNodeAddresses returns the tree node IDs needed to build
// a consistency proof between two specified tree sizes. All the returned nodes
// represent complete subtrees in the tree of size2 or above.
//
// Use Rehash function to compose the proof after the node hashes are fetched.
func CalcConsistencyProofNodeAddresses(size1, size2 int64) (proof.Nodes, error) {
	if size1 < 1 {
		return proof.Nodes{}, status.Errorf(codes.InvalidArgument, "invalid parameter for consistency proof: size1 %d < 1", size1)
	}
	if size2 < 1 {
		return proof.Nodes{}, status.Errorf(codes.InvalidArgument, "invalid parameter for consistency proof: size2 %d < 1", size2)
	}
	if size1 > size2 {
		return proof.Nodes{}, status.Errorf(codes.InvalidArgument, "invalid parameter for consistency proof: size1 %d > size2 %d", size1, size2)
	}
	return proof.Consistency(uint64(size1), uint64(size2)), nil
}
