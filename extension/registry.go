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

package extension

import (
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/storage"
)

// Registry defines all extension points available in Trillian.
// Customizations may easily swap the underlying storage systems by providing their own
// implementation.
type Registry interface {

	// GetLogStorage returns a configured storage.LogStorage instance or an error if the storage cannot be set up.
	GetLogStorage() (storage.LogStorage, error)

	// GetMapStorage returns a configured storage.MapStorage instance or an error if the storage cannot be set up.
	GetMapStorage() (storage.MapStorage, error)

	// GetSigner returns a configured crypto.Signer instance for the specified tree ID or an error if the signer cannot be set up.
	GetSigner(treeID int64) (*crypto.Signer, error)
}
