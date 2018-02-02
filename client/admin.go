// Copyright 2018 Google Inc. All Rights Reserved.
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
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/client/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CreateAndInitTree uses the adminClient and mapClient to create the tree
// described by req.
// If req describes a MAP tree, then this function will also call the InitMap
// function using mapClient.
// Internally, the function will continue to retry failed requests until either
// the tree is created (and if necessary, initialised) successfully, or ctx is
// cancelled.
func CreateAndInitTree(ctx context.Context, req *trillian.CreateTreeRequest, adminClient trillian.TrillianAdminClient, mapClient trillian.TrillianMapClient) (*trillian.Tree, error) {
	b := &backoff.Backoff{
		Min:    100 * time.Millisecond,
		Max:    10 * time.Second,
		Factor: 2,
		Jitter: true,
	}

	var tree *trillian.Tree
	err := b.Retry(ctx, func() error {
		glog.Info("CreateTree...")
		var err error
		tree, err = adminClient.CreateTree(ctx, req)
		if err != nil {
			if s, ok := status.FromError(err); ok && s.Code() == codes.Unavailable {
				glog.Errorf("Admin server unavailable: %v", err)
				return err
			}
			return fmt.Errorf("failed to CreateTree(%+v): %T %v", req, err, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if tree.TreeType == trillian.TreeType_MAP {
		b.Reset()
		err := b.Retry(ctx, func() error {
			glog.Infof("Initialising Map %x...", tree.TreeId)
			// Now, if it's a Map, initialise it.
			req := &trillian.InitMapRequest{MapId: tree.TreeId}
			resp, err := mapClient.InitMap(ctx, req)
			if err != nil {
				switch s, ok := status.FromError(err); {
				case ok && s.Code() == codes.Unavailable:
					glog.Errorf("Map server unavailable: %v", err)
					return err
				case ok && s.Code() == codes.AlreadyExists:
					glog.Warningf("Bizarrely, the just-created Map (%x) is already initialised!: %v", tree.TreeId, err)
					return err
				}
				return fmt.Errorf("failed to InitMap(%+v): %T %v", req, err, err)
			}
			glog.Infof("Initialised Map (%x) with new SignedMapRoot:\n%+v", tree.TreeId, resp.Created)

			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return tree, nil
}
