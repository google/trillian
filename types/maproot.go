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

package types

import (
	"encoding/binary"
	"fmt"

	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/trillian"
)

// MapRootV1 contains the fields verified by SignedMapRoot
type MapRootV1 struct {
	RootHash       []byte `tls:"minlen:0,maxlen:128"`
	TimestampNanos uint64
	Revision       uint64
	Metadata       []byte `tls:"minlen:0,maxlen:65535"`
}

// MapRoot contains the fields serialized into SignedMapRoot.MapRoot
type MapRoot struct {
	Version tls.Enum   `tls:"size:2"`
	V1      *MapRootV1 `tls:"selector:Version,val:1"`
}

// ParseMapRoot verifies that b has the MAP_ROOT_FORMAT_V1 tag and returns a *MapRootV1
func ParseMapRoot(b []byte) (*MapRootV1, error) {
	// Verify version
	version := binary.BigEndian.Uint16(b)
	if version != uint16(trillian.MapRootFormat_MAP_ROOT_FORMAT_V1) {
		return nil, fmt.Errorf("invalid MapRoot.Version: %v, want %v",
			version, trillian.MapRootFormat_MAP_ROOT_FORMAT_V1)
	}

	var logRoot MapRoot
	if _, err := tls.Unmarshal(b, &logRoot); err != nil {
		return nil, err
	}
	return logRoot.V1, nil
}

// SerializeMapRoot returns a canonical TLS serialization of the map root.
func SerializeMapRoot(r *MapRootV1) ([]byte, error) {
	return tls.Marshal(MapRoot{
		Version: tls.Enum(trillian.MapRootFormat_MAP_ROOT_FORMAT_V1),
		V1:      r,
	})
}
