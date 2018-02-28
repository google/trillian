// Copyright 2017 Google Inc. All Rights Reserved.
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

package crypto

import (
	"fmt"

	"github.com/google/certificate-transparency-go/tls"

	"github.com/google/trillian"
)

// This file contains struct specific mappings and data structures.

// SignedLogRootV1 contains the fields verified by SignedLogRootV1
type SignedLogRootV1 struct {
	DataFormatVersion uint32
	RootHash          []byte `tls:"minlen:0,maxlen:128"`
	TimestampNanos    uint64
	TreeSize          uint64
	LogID             uint64
}

// SignedMapRootV1 contains the fields verified by SignedMapRootV1
type SignedMapRootV1 struct {
	DataFormatVersion uint32
	RootHash          []byte `tls:"minlen:0,maxlen:128"`
	TimestampNanos    uint64
	MapID             uint64
	MapRevision       uint64
	MetadataType      []byte `tls:"minlen:0,maxlen:65535"`
	MetadataValue     []byte `tls:"minlen:0,maxlen:65535"`
}

// SerializeLogRoot returns a canonical TLS serialization of the log root.
func SerializeLogRoot(r *trillian.SignedLogRoot, version trillian.LogSignatureFormat) ([]byte, error) {
	switch version {
	case trillian.LogSignatureFormat_LOG_SIG_FORMAT_V1:
		root := SignedLogRootV1{
			DataFormatVersion: uint32(version),
			RootHash:          r.RootHash,
			TimestampNanos:    uint64(r.TimestampNanos),
			TreeSize:          uint64(r.TreeSize),
			LogID:             uint64(r.LogId),
		}
		return tls.Marshal(root)
	default:
		return nil, fmt.Errorf("crypto: CanonicalLogRoot(): unknown version: %v", version)
	}
}

// SerializeMapRoot returns a canonical TLS serialization of the map root.
func SerializeMapRoot(r *trillian.SignedMapRoot, version trillian.MapSignatureFormat) ([]byte, error) {
	switch version {
	case trillian.MapSignatureFormat_MAP_SIG_FORMAT_V1:
		root := SignedMapRootV1{
			DataFormatVersion: uint32(version),
			RootHash:          r.RootHash,
			TimestampNanos:    uint64(r.TimestampNanos),
			MapID:             uint64(r.MapId),
			MapRevision:       uint64(r.MapRevision),
			MetadataType:      []byte(r.Metadata.GetTypeUrl()),
			MetadataValue:     r.Metadata.GetValue(),
		}
		return tls.Marshal(root)

	default:
		return nil, fmt.Errorf("crypto: CanonicalLogRoot(): unknown version: %v", version)
	}
}
