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

package hammer

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
)

func destroyMap(ctx context.Context, adminClient trillian.TrillianAdminClient, mapID int64) error {
	req := &trillian.DeleteTreeRequest{TreeId: mapID}
	glog.Infof("Soft-delete transient Trillian Map with TreeID=%d", mapID)
	if _, err := adminClient.DeleteTree(ctx, req); err != nil {
		return fmt.Errorf("failed to DeleteTree(%d): %v", mapID, err)
	}
	return nil
}

func makeNewMap(ctx context.Context, adminClient trillian.TrillianAdminClient, mapClient trillian.TrillianMapClient) (int64, error) {
	nowSec := time.Now().UnixNano() / int64(time.Second)
	req := &trillian.CreateTreeRequest{
		Tree: &trillian.Tree{
			TreeState:          trillian.TreeState_ACTIVE,
			TreeType:           trillian.TreeType_MAP,
			HashStrategy:       trillian.HashStrategy_TEST_MAP_HASHER,
			HashAlgorithm:      sigpb.DigitallySigned_SHA256,
			SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
			DisplayName:        fmt.Sprintf("maphammer-%d", nowSec),
			Description:        "Transient tree for MapHammer test",
			MaxRootDuration:    ptypes.DurationProto(time.Second * 3600),
		},
		KeySpec: &keyspb.Specification{
			Params: &keyspb.Specification_EcdsaParams{
				EcdsaParams: &keyspb.Specification_ECDSA{
					Curve: keyspb.Specification_ECDSA_P256,
				},
			},
		},
	}

	tree, err := client.CreateAndInitTree(ctx, req, adminClient, mapClient, nil)
	if err != nil {
		return -1, fmt.Errorf("client.CreateAndInitTree(%v) failed with err: %v", req, err)
	}
	glog.Infof("Made new Trillian Map with TreeID=%d", tree.TreeId)

	return tree.TreeId, nil
}
