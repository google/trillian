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
