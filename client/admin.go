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

// CreateAndInitTree uses the adminClient and logClient/mapClient to create the tree
// described by req.
// If req describes a MAP tree, then this function will also call the InitMap
// function using mapClient.
// If req describes a LOG tree, then this function will also call the InitLog
// function using logClient.
// Internally, the function will continue to retry failed requests until either
// the tree is created (and if necessary, initialised) successfully, or ctx is
// cancelled.
func CreateAndInitTree(
	ctx context.Context,
	req *trillian.CreateTreeRequest,
	adminClient trillian.TrillianAdminClient,
	mapClient trillian.TrillianMapClient,
	logClient trillian.TrillianLogClient) (*trillian.Tree, error) {

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
			glog.Errorf("failed to CreateTree(%+v): %T %v", req, err, err)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	switch tree.TreeType {
	case trillian.TreeType_MAP:
		if err := InitMap(ctx, tree, mapClient); err != nil {
			return nil, err
		}
	case trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG:
		if err := InitLog(ctx, tree, logClient); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("Don't know how or whether to initialise tree type %v", tree.TreeType)
	}

	return tree, nil
}

// InitMap initialises a freshly created Map tree.
func InitMap(ctx context.Context, tree *trillian.Tree, mapClient trillian.TrillianMapClient) error {
	if tree.TreeType != trillian.TreeType_MAP {
		return fmt.Errorf("InitMap called with tree of type %v", tree.TreeType)
	}

	b := &backoff.Backoff{
		Min:    100 * time.Millisecond,
		Max:    10 * time.Second,
		Factor: 2,
		Jitter: true,
	}

	err := b.Retry(ctx, func() error {
		glog.Infof("Initialising Map %x...", tree.TreeId)
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
			glog.Errorf("failed to InitMap(%+v): %T %v", req, err, err)
			return err
		}
		glog.Infof("Initialised Map (%x) with new SignedMapRoot:\n%+v", tree.TreeId, resp.Created)

		return nil
	})
	if err != nil {
		return err
	}

	// Wait for map root to become available.
	return b.Retry(ctx, func() error {
		_, err := mapClient.GetSignedMapRootByRevision(ctx,
			&trillian.GetSignedMapRootByRevisionRequest{
				MapId:    tree.TreeId,
				Revision: 0,
			})
		return err
	})
}

// InitLog initialises a freshly created Log tree.
func InitLog(ctx context.Context, tree *trillian.Tree, logClient trillian.TrillianLogClient) error {
	if tree.TreeType != trillian.TreeType_LOG &&
		tree.TreeType != trillian.TreeType_PREORDERED_LOG {
		return fmt.Errorf("InitLog called with tree of type %v", tree.TreeType)
	}

	b := &backoff.Backoff{
		Min:    100 * time.Millisecond,
		Max:    10 * time.Second,
		Factor: 2,
		Jitter: true,
	}

	err := b.Retry(ctx, func() error {
		glog.Infof("Initialising Log %x...", tree.TreeId)
		req := &trillian.InitLogRequest{LogId: tree.TreeId}
		resp, err := logClient.InitLog(ctx, req)
		if err != nil {
			switch s, ok := status.FromError(err); {
			case ok && s.Code() == codes.Unavailable:
				glog.Errorf("Log server unavailable: %v", err)
				return err
			case ok && s.Code() == codes.AlreadyExists:
				glog.Warningf("Bizarrely, the just-created Log (%x) is already initialised!: %v", tree.TreeId, err)
				return err
			}
			glog.Errorf("failed to InitLog(%+v): %T %v", req, err, err)
			return err
		}
		glog.Infof("Initialised Log (%x) with new SignedTreeHead:\n%+v", tree.TreeId, resp.Created)

		return nil
	})
	if err != nil {
		return err
	}

	// Wait for log root to become available.
	return b.Retry(ctx, func() error {
		_, err := logClient.GetLatestSignedLogRoot(ctx,
			&trillian.GetLatestSignedLogRootRequest{LogId: tree.TreeId})
		return err
	})
}
