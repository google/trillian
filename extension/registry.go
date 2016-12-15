package extension

import (
	"github.com/google/trillian/storage"
)

// Registry defines all extension points available in Trillian.
// Customizations may easily swap the underlying storage systems by providing their own
// implementation.
type Registry interface {

	// GetLogStorage returns a configured storage.LogStorage instance for the specified tree ID or an
	// error if the storage cannot be setup.
	GetLogStorage(treeID int64) (storage.LogStorage, error)

	// GetMapStorage returns a configured storage.MapStorage instance for the specified tree ID or an
	// error if the storage cannot be setup.
	GetMapStorage(treeID int64) (storage.MapStorage, error)
}
