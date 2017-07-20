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

package crypto

import (
	"crypto"
	"encoding/json"

	"github.com/benlaurie/objecthash/go/objecthash"
	"github.com/google/trillian/crypto/sigpb"
)

// SignObject signs the requested object using ObjectHash.
func (s *Signer) SignObject(obj interface{}) (*sigpb.DigitallySigned, error) {
	j, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	hash := objecthash.CommonJSONHash(string(j))
	return s.Sign(hash[:])
}

// VerifyObject verifies the output of Signer.SignObject.
func VerifyObject(pub crypto.PublicKey, obj interface{}, sig *sigpb.DigitallySigned) error {
	j, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	hash := objecthash.CommonJSONHash(string(j))

	return Verify(pub, hash[:], sig)
}
