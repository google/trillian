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

package client

import (
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"github.com/transparency-dev/merkle/rfc6962"
)

func TestVerifyRootErrors(t *testing.T) {
	logRoot, err := (&types.LogRootV1{}).MarshalBinary()
	if err != nil {
		t.Fatalf("Failed to create test signature: %v", err)
	}
	signedRoot := &trillian.SignedLogRoot{LogRoot: logRoot}

	// Test execution
	tests := []struct {
		desc    string
		trusted *types.LogRootV1
		newRoot *trillian.SignedLogRoot
	}{
		{desc: "newRootNil", trusted: &types.LogRootV1{}, newRoot: nil},
		{desc: "trustedNil", trusted: nil, newRoot: signedRoot},
	}
	for _, test := range tests {
		logVerifier := NewLogVerifier(rfc6962.DefaultHasher)

		// This also makes sure that no nil pointer dereference errors occur (as this would cause a panic).
		if _, err := logVerifier.VerifyRoot(test.trusted, test.newRoot, nil); err == nil {
			t.Errorf("%v: VerifyRoot() error expected, but got nil", test.desc)
		}
	}
}

func TestVerifyInclusionByHashErrors(t *testing.T) {
	tests := []struct {
		desc    string
		trusted *types.LogRootV1
		proof   *trillian.Proof
	}{
		{desc: "trustedNil", trusted: nil, proof: &trillian.Proof{}},
		{desc: "proofNil", trusted: &types.LogRootV1{}, proof: nil},
	}
	for _, test := range tests {

		logVerifier := NewLogVerifier(nil)
		err := logVerifier.VerifyInclusionByHash(test.trusted, nil, test.proof)
		if err == nil {
			t.Errorf("%v: VerifyInclusionByHash() error expected, but got nil", test.desc)
		}
	}
}
