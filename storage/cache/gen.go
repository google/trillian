package cache

//go:generate mockgen -self_package github.com/google/trillian/storage/cache -package cache -destination mock_node_storage.go github.com/google/trillian/storage/cache NodeStorage

import (
	"github.com/google/trillian/storage"
)

// NodeStorage provides an interface for storing and retrieving subtrees.
type NodeStorage interface {
	GetSubtree(n storage.NodeID) (*storage.SubtreeProto, error)
	SetSubtrees(s []*storage.SubtreeProto) error
}
