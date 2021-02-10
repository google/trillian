// Copyright 2017 Google LLC. All Rights Reserved.
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

package testonly

import (
	"log"
	"testing"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
)

// MustMarshalAnyNoT is used to Marshal proto messages into the
// protobuf.ptypes.any.Any used throughout the Trillian API and in
// storage.  Use if testing.T not available. Failure to marshal will
// fail the test suite.
func MustMarshalAnyNoT(in proto.Message) []byte {
	protoBytes, err := proto.Marshal(in)
	if err != nil {
		log.Fatalf("failed to marshal %v as 'bytes': err %v", in, err)
	}
	return protoBytes
}

// MustMarshalAny is used in tests to Marshal proto messages into the
// protobuf.ptypes.any.Any used in the Trillian API and in storage.
// Failure to marshal will fail the test but the suite will continue.
func MustMarshalAny(t *testing.T, in proto.Message) *any.Any {
	t.Helper()
	anything, err := ptypes.MarshalAny(in)
	if err != nil {
		t.Fatalf("failed to marshal %v as 'any': err %v", in, err)
	}
	return anything
}
