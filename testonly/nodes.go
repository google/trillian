package testonly

import (
	"github.com/google/trillian/storage"
)

func MustCreateNodeIDForTreeCoords(depth, index int64, maxPathBits int) storage.NodeID {
	n, err := storage.NewNodeIDForTreeCoords(depth, index, maxPathBits)
	if err != nil {
		panic(err)
	}
	return n
}
