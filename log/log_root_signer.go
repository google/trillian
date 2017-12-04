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

package log

import (
	"github.com/golang/glog"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/sigpb"
)

// CreateAndSignLogRoot creates a to-be-SignedLogRoot proto given the func arguments,
// signs and returns it.
func CreateAndSignLogRoot(signer *crypto.Signer, rootHash []byte, tsNanos, treeSize, logID, revision int64) (*trillian.SignedLogRoot, error) {
	logRoot := &trillian.SignedLogRoot{
		RootHash:       rootHash,
		TimestampNanos: tsNanos,
		TreeSize:       treeSize,
		LogId:          logID,
		TreeRevision:   revision,
	}

	// Hash and sign the root.
	signature, err := CreateLogRootSignature(signer, logRoot)
	if err != nil {
		return nil, err
	}
	logRoot.Signature = signature
	return logRoot, nil
}

// CreateLogRootSignature hashes and signs the supplied (to-be) SignedLogRoot and returns the signature.
func CreateLogRootSignature(signer *crypto.Signer, root *trillian.SignedLogRoot) (*sigpb.DigitallySigned, error) {
	hash, err := crypto.HashLogRoot(*root)
	if err != nil {
		return nil, err
	}
	signature, err := signer.Sign(hash)
	if err != nil {
		glog.Warningf("%v: signer failed to sign root: %v", root.LogId, err)
		return nil, err
	}

	return signature, nil
}
