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

package keys

import (
	"context"
	"crypto"
	"fmt"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
)

// PEMSignerFactory loads PEM-encoded private keys.
// It only supports trees whose PrivateKey field is a trillian.PEMKeyFile.
// It implements keys.SignerFactory.
// TODO(robpercival): Should this cache loaded private keys? The SequenceManager will request a signer for each batch of leaves it sequences.
type PEMSignerFactory struct{}

// NewSigner returns a crypto.Signer for the given tree.
func (f PEMSignerFactory) NewSigner(ctx context.Context, tree *trillian.Tree) (crypto.Signer, error) {
	if tree.GetPrivateKey() == nil {
		return nil, fmt.Errorf("tree %d has no PrivateKey", tree.GetTreeId())
	}

	var privateKey ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(tree.GetPrivateKey(), &privateKey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal private key for tree %d: %v", tree.GetTreeId(), err)
	}

	switch privateKey := privateKey.Message.(type) {
	case *trillian.PEMKeyFile:
		return NewFromPrivatePEMFile(privateKey.GetPath(), privateKey.GetPassword())
	}

	return nil, fmt.Errorf("unsupported PrivateKey type for tree %d: %T", tree.GetTreeId(), privateKey.Message)
}
