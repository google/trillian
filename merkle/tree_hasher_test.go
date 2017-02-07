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

package merkle

import (
	"bytes"
	"testing"

	"github.com/google/trillian/testonly"
)

const (
	// echo -n | sha256sum
	rfc6962EmptyHashHex = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	// echo -n 004C313233343536 | xxd -r -p | sha256sum
	rfc6962LeafL123456HashHex = "395aa064aa4c29f7010acfe3f25db9485bbd4b91897b6ad7ad547639252b4d56"
	// echo -n 014E3132334E343536 | xxd -r -p | sha256sum
	rfc6962NodeN123N456HashHex = "aa217fe888e47007fa15edab33c2b492a722cb106c64667fc2b044444de66bbb"
)

func ensureHashMatches(expected, actual []byte, testCase string, t *testing.T) {
	if bytes.Compare(expected, actual) != 0 {
		t.Fatalf("Hash mismatch testing %s expected %v, got: %v", testCase, expected, actual)
	}
}

func TestRfc6962Hasher(t *testing.T) {
	hasher := testonly.Hasher

	ensureHashMatches(testonly.MustHexDecode(rfc6962EmptyHashHex), hasher.HashEmpty(), "RFC962 Empty", t)
	ensureHashMatches(testonly.MustHexDecode(rfc6962LeafL123456HashHex), hasher.HashLeaf([]byte("L123456")), "RFC6962 Leaf", t)
	ensureHashMatches(testonly.MustHexDecode(rfc6962NodeN123N456HashHex), hasher.HashChildren([]byte("N123"), []byte("N456")), "RFC6962 Node", t)
}
