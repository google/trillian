//go:build !pkcs11
// +build !pkcs11

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

// Package pkcs11 provides access to private keys using a PKCS#11 interface.
package pkcs11

import (
	"crypto"
	"errors"

	"github.com/google/trillian/crypto/keyspb"
)

// FromConfig returns an error indicating that PKCS11 is not supported.
func FromConfig(_ string, _ *keyspb.PKCS11Config) (crypto.Signer, error) {
	return nil, errors.New("pkcs11: Not supported in this binary")
}
