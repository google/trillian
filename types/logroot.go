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

// LogRootV1 contains the fields used by clients to verify Trillian Log behavior.
type LogRootV1 struct {
	TreeSize       uint64
	RootHash       []byte `tls:"minlen:0,maxlen:128"`
	TimestampNanos uint64
	Revision       uint64
	Metadata       []byte `tls:"minlen:0,maxlen:65535"`
}

// LogRoot contains the fields serialized into SignedLogRoot.LogRoot
type LogRoot struct {
	Version tls.Enum   `tls:"size:2"`
	V1      *LogRootV1 `tls:"selector:Version,val:1"`
}

// ParseLogRoot verifies that b has the LOG_ROOT_FORMAT_V1 tag and returns a *LogRootV1
func ParseLogRoot(b []byte) (*LogRootV1, error) {
	// Verify version
	version := binary.BigEndian.Uint16(b)
	if version != uint16(trillian.LogRootFormat_LOG_ROOT_FORMAT_V1) {
		return nil, fmt.Errorf("invalid LogRoot.Version: %v, want %v",
			version, trillian.LogRootFormat_LOG_ROOT_FORMAT_V1)
	}

	var logRoot LogRoot
	if _, err := tls.Unmarshal(b, &logRoot); err != nil {
		return nil, err
	}
	return logRoot.V1, nil
}

// SerializeLogRoot returns a canonical TLS serialization of the log root.
func SerializeLogRoot(r *LogRootV1) ([]byte, error) {
	return tls.Marshal(LogRoot{
		Version: tls.Enum(trillian.LogRootFormat_LOG_ROOT_FORMAT_V1),
		V1:      r,
	})
}
