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
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"

	"github.com/google/certificate-transparency-go/tls"

	"github.com/benlaurie/objecthash/go/objecthash"
	"github.com/google/trillian"
)

// This file contains struct specific mappings and data structures.

// Constants used as map keys when building input for ObjectHash. They must not be changed
// as this will change the output of hashRoot()
const (
	mapKeyRootHash       string = "RootHash"
	mapKeyTimestampNanos string = "TimestampNanos"
	mapKeyTreeSize       string = "TreeSize"
)

// hashLogRoot hashes SignedLogRoot objects using ObjectHash with
// "RootHash", "TimestampNanos", and "TreeSize", used as keys in
// a map.
func hashLogRoot(root trillian.SignedLogRoot) ([]byte, error) {
	// Pull out the fields we want to hash.
	// Caution: use string format for int64 values as they can overflow when
	// JSON encoded otherwise (it uses floats). We want to be sure that people
	// using JSON to verify hashes can build the exact same input to ObjectHash.
	rootMap := map[string]interface{}{
		mapKeyRootHash:       base64.StdEncoding.EncodeToString(root.RootHash),
		mapKeyTimestampNanos: strconv.FormatInt(root.TimestampNanos, 10),
		mapKeyTreeSize:       strconv.FormatInt(root.TreeSize, 10)}

	hash, err := objecthash.ObjectHash(rootMap)
	if err != nil {
		return nil, fmt.Errorf("ObjectHash(%#v): %v", rootMap, err)
	}
	return hash[:], nil
}

// LogRootV1 contains the fields verified by SignedLogRoot
type LogRootV1 struct {
	Version        uint16
	TreeSize       uint64
	RootHash       []byte `tls:"minlen:0,maxlen:128"`
	TimestampNanos uint64
	Revision       uint64
	Metadata       []byte `tls:"minlen:0,maxlen:65535"`
}

// MapRootV1 contains the fields verified by SignedMapRoot
type MapRootV1 struct {
	Version        uint16
	RootHash       []byte `tls:"minlen:0,maxlen:128"`
	TimestampNanos uint64
	Revision       uint64
	Metadata       []byte `tls:"minlen:0,maxlen:65535"`
}

// ParseLogRoot returns a *SignedLogRootV1
func ParseLogRoot(b []byte) (*LogRootV1, error) {
	// Verify version
	version := binary.BigEndian.Uint16(b)
	if got, want := version, uint16(trillian.LogRootFormat_LOG_ROOT_FORMAT_V1); got != want {
		return nil, fmt.Errorf("invalid LogRoot.Version: %v, want %v", got, want)
	}

	var logRoot LogRootV1
	if _, err := tls.Unmarshal(b, &logRoot); err != nil {
		return nil, err
	}
	return &logRoot, nil
}

// SerializeLogRoot returns a canonical TLS serialization of the log root.
func SerializeLogRoot(r *LogRootV1) ([]byte, error) {
	root := *r
	root.Version = uint16(trillian.LogRootFormat_LOG_ROOT_FORMAT_V1)
	return tls.Marshal(root)
}

// ParseMapRoot returns a *SignedMapRootV1
func ParseMapRoot(b []byte) (*MapRootV1, error) {
	// Verify version
	version := binary.BigEndian.Uint16(b)
	if got, want := version, uint16(trillian.MapRootFormat_MAP_ROOT_FORMAT_V1); got != want {
		return nil, fmt.Errorf("invalid MapRoot.Version: %v, want %v", got, want)
	}

	var logRoot MapRootV1
	if _, err := tls.Unmarshal(b, &logRoot); err != nil {
		return nil, err
	}
	return &logRoot, nil
}

// SerializeMapRoot returns a canonical TLS serialization of the map root.
func SerializeMapRoot(r *MapRootV1) ([]byte, error) {
	root := *r
	root.Version = uint16(trillian.MapRootFormat_MAP_ROOT_FORMAT_V1)
	return tls.Marshal(root)
}
