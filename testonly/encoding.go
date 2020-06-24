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

package testonly

import (
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
)

// MustDecodeBase64 expects a base 64 encoded string input and panics if it cannot be decoded
func MustDecodeBase64(b64 string) []byte {
	r, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(r)
	}
	return r
}

// MustHexDecode decodes its input string from hex and panics if this fails
func MustHexDecode(b string) []byte {
	r, err := hex.DecodeString(b)
	if err != nil {
		panic(err)
	}
	return r
}

// MustToTimestampProto converts t to a Timestamp protobuf, or panics if this fails.
func MustToTimestampProto(t time.Time) *tspb.Timestamp {
	ret, err := ptypes.TimestampProto(t)
	if err != nil {
		panic(err)
	}
	return ret
}
