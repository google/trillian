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

	// defMapStrata describes the default set of subtree depths for use by
	// Maps.
	defMapStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 176}
)
