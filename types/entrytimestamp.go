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
	"errors"
	"fmt"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/trillian"
)

// EntryTimestampV1 holds the TLS-deserialization of the following structure
// (described in RFC5246 section 4 notation):
// struct {
//   uint64 timestamp_nanos;
//   opaque leaf_identity_hash<0..65535>;
//   opaque merkle_leaf_hash<0..65535>;
// } EntryTimestampV1;
type EntryTimestampV1 struct {
	TimestampNanos   uint64
	LeafIdentityHash []byte `tls:"minlen:0,maxlen:65535"`
	MerkleLeafHash   []byte `tls:"minlen:0,maxlen:65535"`
}

// EntryTimestamp holds the TLS-deserialization of the following structure
// (described in RFC5246 section 4 notation):
// enum { v1(1), (65535)} Version;
// struct {
//   Version version;
//   select(version) {
//     case v1: EntryTimestampV1;
//   }
// } EntryTimestamp;
type EntryTimestamp struct {
	Version tls.Enum          `tls:"size:2"`
	V1      *EntryTimestampV1 `tls:"selector:Version,val:1"`
}

// UnmarshalBinary verifies that etBytes is a TLS-serialized EntryTimestamp, has
// the ENTRY_TIMESTAMP_FORMAT_V1 tag, and populates the invoked object with the
// deserialized *EntryTimestampV1.
func (l *EntryTimestampV1) UnmarshalBinary(etBytes []byte) error {
	if etBytes == nil {
		return errors.New("nil entry timestamp")
	}
	version := binary.BigEndian.Uint16(etBytes)
	if version != uint16(trillian.EntryTimestampFormat_ENTRY_TIMESTAMP_FORMAT_V1) {
		return fmt.Errorf("invalid EntryTimestamp.Version: %v, want %v",
			version, trillian.EntryTimestampFormat_ENTRY_TIMESTAMP_FORMAT_V1)
	}

	var et EntryTimestamp
	if rest, err := tls.Unmarshal(etBytes, &et); err != nil {
		return err
	} else if len(rest) > 0 {
		return fmt.Errorf("trailing data (%d bytes) in encoded data", len(rest))
	}

	*l = *et.V1
	return nil
}

// MarshalBinary returns a canonical TLS serialization of EntryTimestamp.
func (l *EntryTimestampV1) MarshalBinary() ([]byte, error) {
	return tls.Marshal(EntryTimestamp{
		Version: tls.Enum(trillian.EntryTimestampFormat_ENTRY_TIMESTAMP_FORMAT_V1),
		V1:      l,
	})
}

// EntryDataForLeaf returns a canonical TLS serialization of the EntryTimestamp
// that would be generated for a given leaf.
func EntryDataForLeaf(leaf *trillian.LogLeaf) ([]byte, error) {
	ts, err := ptypes.Timestamp(leaf.QueueTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %v", err)
	}
	et := EntryTimestampV1{
		TimestampNanos:   uint64(ts.UnixNano()),
		LeafIdentityHash: leaf.LeafIdentityHash,
		MerkleLeafHash:   leaf.MerkleLeafHash,
	}
	etData, err := et.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal EntryTimestamp: %v", err)
	}
	return etData, nil
}
