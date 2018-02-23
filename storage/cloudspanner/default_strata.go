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

package cloudspanner

// Default strata sizes for the different tree types.
var (
	// defLogStrata is a suitable set of stratum sizes for Log trees.
	// Log trees are dense and so each individual stratum cannot over-commit on
	// storage.
	defLogStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8}

	// defMapStrata is a suitable set of strata sizes for Map trees built with
	// 256-bit hashes.
	// Map trees are sparse, and while the top n levels of the tree will end up
	// being quite densely populated (n<=80 here), the subtrees below are
	// unlikely to have more than one or two leaf nodes in them, so we can save
	// some space and time by having a large bottom stratum.
	defMapStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 176}
)
