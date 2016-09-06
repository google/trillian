package testonly

import (
	"github.com/google/trillian/storage"
)

func MustCreateNodeIDForTreeCoords(depth, index int64, maxBitLen int) storage.NodeID {
	n, err := storage.NewNodeIDForTreeCoords(depth, index, maxBitLen)
	if err != nil {
		panic(err)
	}
	return n
}
