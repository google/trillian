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

package testonly

import (
	"crypto/x509"

	"github.com/google/trillian/crypto/keys"
)

// MustMarshalPublicPEMToDER reads a PEM-encoded public key and returns it in DER encoding.
// If an error occurs, it panics.
func MustMarshalPublicPEMToDER(pem string) []byte {
	key, err := keys.NewFromPublicPEM(pem)
	if err != nil {
		panic(err)
	}

	keyDER, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		panic(err)
	}
	return keyDER
}

// MustMarshalPrivatePEMToDER decrypts a PEM-encoded private key and returns it in DER encoding.
// If an error occurs, it panics.
func MustMarshalPrivatePEMToDER(pem, password string) []byte {
	key, err := keys.NewFromPrivatePEM(pem, password)
	if err != nil {
		panic(err)
	}

	keyDER, err := keys.MarshalPrivateKey(key)
	if err != nil {
		panic(err)
	}
	return keyDER
}
