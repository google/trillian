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

// Package extension provides an extension mechanism for Trillian code to access
// fork-specific functionality.
package extension

import (
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util/election2"
)

// Registry defines all extension points available in Trillian.
// Customizations may easily swap the underlying storage systems by providing their own
// implementation.
type Registry struct {
	// AdminStorage is the storage implementation to use for persisting tree metadata.
	storage.AdminStorage
	// LogStorage is the storage implementation to use for persisting logs.
	storage.LogStorage
	// MapStorage is the storage implementation to use for persisting maps.
	storage.MapStorage
	// ElectionFactory provides Election instances for each tree.
	ElectionFactory election2.Factory
	// QuotaManager provides rate limiting capabilities for Trillian.
	QuotaManager quota.Manager
	// MetricFactory provides metrics for monitoring.
	monitoring.MetricFactory
	// NewKeyProto creates a new private key based on a key specification.
	// It returns a proto that can be passed to a keys.ProtoHandler to get a crypto.Signer.
	NewKeyProto keys.ProtoGenerator
	// SetProcessStatus sets the current process status for diagnostic purposes.
	SetProcessStatus func(string)
}
