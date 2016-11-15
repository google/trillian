package cache

//go:generate mockgen -self_package github.com/google/trillian/storage/cache -package cache -imports github.com/google/trillian/storage/proto -destination mock_node_storage.go github.com/google/trillian/storage/cache NodeStorage

import (
	"github.com/google/trillian/storage"
	storagepb "github.com/google/trillian/storage/proto"
)

// NodeStorage provides an interface for storing and retrieving subtrees.
type NodeStorage interface {
	GetSubtree(n storage.NodeID) (*storagepb.SubtreeProto, error)
	SetSubtrees(s []*storagepb.SubtreeProto) error
}
