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
	"testing"

	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/testonly"
)

func TestSignVerifyObject(t *testing.T) {
	key, err := keys.NewFromPrivatePEM(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSHA256Signer(key)

	type subfield struct {
		c int
	}

	for _, tc := range []struct {
		obj interface{}
	}{
		{obj: struct{ a string }{a: "foo"}},
		{obj: struct {
			a int
			b *subfield
		}{a: 1, b: &subfield{c: 0}}},
		{obj: struct {
			a int
			b *subfield
		}{a: 1, b: nil}},
	} {
		sig, err := signer.SignObject(tc.obj)
		if err != nil {
			t.Errorf("SignObject(%#v): %v", tc.obj, err)
			continue
		}
		if err := VerifyObject(key.Public(), tc.obj, sig); err != nil {
			t.Errorf("SignObject(%#v): %v", tc.obj, err)
		}
	}
}
