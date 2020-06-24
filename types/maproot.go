// Copyright 2018 Google LLC. All Rights Reserved.
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

// MapRootV1 holds the TLS-deserialization of the following structure
// (described in RFC5246 section 4 notation):
// struct {
//   opaque root_hash<0..128>;
//   uint64 timestamp_nanos;
//   uint64 revision;
//   opaque metadata<0..65535>;
// } MapRootV1;
type MapRootV1 struct {
	RootHash       []byte `tls:"minlen:0,maxlen:128"`
	TimestampNanos uint64
	Revision       uint64
	Metadata       []byte `tls:"minlen:0,maxlen:65535"`
}

// MapRoot holds the TLS-deserialization of the following structure
// (described in RFC5246 section 4 notation):
// enum { v1(1), (65535)} Version;
// struct {
//   Version version;
//   select(version) {
//     case v1: MapRootV1;
//   }
// } MapRoot;
type MapRoot struct {
	Version tls.Enum   `tls:"size:2"`
	V1      *MapRootV1 `tls:"selector:Version,val:1"`
}

// UnmarshalBinary verifies that mapRootBytes is a TLS serialized MapRoot,
// has the MAP_ROOT_FORMAT_V1 tag, and returns the deserialized *MapRootV1.
func (m *MapRootV1) UnmarshalBinary(mapRootBytes []byte) error {
	if len(mapRootBytes) < 3 {
		return fmt.Errorf("mapRootBytes too short")
	}
	if m == nil {
		return fmt.Errorf("nil map root")
	}
	version := binary.BigEndian.Uint16(mapRootBytes)
	if version != uint16(trillian.MapRootFormat_MAP_ROOT_FORMAT_V1) {
		return fmt.Errorf("invalid MapRoot.Version: %v, want %v",
			version, trillian.MapRootFormat_MAP_ROOT_FORMAT_V1)
	}

	var mapRoot MapRoot
	if _, err := tls.Unmarshal(mapRootBytes, &mapRoot); err != nil {
		return err
	}
	*m = *mapRoot.V1
	return nil
}

// MarshalBinary returns a canonical TLS serialization of the map root.
func (m *MapRootV1) MarshalBinary() ([]byte, error) {
	return tls.Marshal(MapRoot{
		Version: tls.Enum(trillian.MapRootFormat_MAP_ROOT_FORMAT_V1),
		V1:      m,
	})
}
