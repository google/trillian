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

// Package proto registers a DER keys.ProtoHandler using keys.RegisterHandler.
// This handler will extract a crypto.Signer from a keyspb.PrivateKey protobuf message.
package proto

import (
	"context"
	"crypto"
	"fmt"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keyspb"
)

func init() {
	keys.RegisterHandler(&keyspb.PrivateKey{}, func(ctx context.Context, pb proto.Message) (crypto.Signer, error) {
		if pb, ok := pb.(*keyspb.PrivateKey); ok {
			return der.FromProto(pb)
		}
		return nil, fmt.Errorf("der: got %T, want *keyspb.PrivateKey", pb)
	})
}
