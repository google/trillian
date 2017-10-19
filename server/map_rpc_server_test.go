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

package server

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/kylelemons/godebug/pretty"
)

const (
	mapID1 = int64(1)
)

func TestIsHealthy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		desc string
		accessibleErr error
		wantErr bool
	}{
		{"healthy", nil, false},
		{"unhealthy", errors.New("DB not happy"), true},
	}

	for _, test := range tests {
		mockStorage := storage.NewMockMapStorage(ctrl)
		mockStorage.EXPECT().CheckDatabaseAccessible(gomock.Any()).Return(test.accessibleErr)

		server := NewTrillianMapServer(extension.Registry{
			AdminStorage: mockAdminStorageForMap(ctrl, mapID1),
			MapStorage:   mockStorage,
		})

		err := server.IsHealthy()
		if gotErr := err != nil; gotErr != test.wantErr {
			t.Errorf("%s: IsHealthy() err? %t want? %t (err=%v)", test.desc, gotErr, test.wantErr, err)
		}
	}
}

func TestGetSignedMapRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	tests := []struct {
		desc string
		req *trillian.GetSignedMapRootRequest
		mapRoot trillian.SignedMapRoot
		snapShErr, lsmrErr error
		wantErr bool
	}{
		{
			desc: "Unknown map",
			req: &trillian.GetSignedMapRootRequest{},
			snapShErr: errors.New("unknown map"),
			wantErr: true,
		},
		{
			desc: "Map is empty, head at revision 0",
			req: &trillian.GetSignedMapRootRequest{MapId: mapID1},
			lsmrErr: errors.New("sql: no rows in result set"),
			wantErr: true,
		},
		{
			desc: "Map has leaves, head > revision 0",
			req: &trillian.GetSignedMapRootRequest{MapId: mapID1},
			mapRoot: trillian.SignedMapRoot{
			},
		},
	}

	for _, test := range tests {
		mockStorage := storage.NewMockMapStorage(ctrl)
		mockTx := storage.NewMockMapTreeTX(ctrl)

		mockStorage.EXPECT().SnapshotForTree(gomock.Any(), test.req.MapId).Return(mockTx, test.snapShErr)
		if test.snapShErr == nil {
			mockTx.EXPECT().LatestSignedMapRoot(gomock.Any()).Return(test.mapRoot, test.lsmrErr)
			if test.lsmrErr == nil {
				mockTx.EXPECT().Commit().Return(nil)
			}
			mockTx.EXPECT().Close().Return(nil)
			mockTx.EXPECT().IsOpen().AnyTimes().Return(false)
		}

		server := NewTrillianMapServer(extension.Registry{
			AdminStorage: mockAdminStorageForMap(ctrl, mapID1),
			MapStorage:   mockStorage,
		})

		smrResp, err := server.GetSignedMapRoot(ctx, test.req)

		if gotErr := err != nil; gotErr != test.wantErr {
			t.Errorf("%s: GetSignedMapRoot()=_, err? %t want? %t (err=%v)", test.desc, gotErr, test.wantErr, err)
		}
		if err != nil {
			continue
		}
		want := &trillian.GetSignedMapRootResponse{MapRoot: &test.mapRoot}
		if got := smrResp; !proto.Equal(got, want) {
			diff := pretty.Compare(got, want)
			t.Errorf("%s: GetSignedMapRoot() got != want, diff:\n%v", test.desc, diff)
		}
	}
}

func mockAdminStorageForMap(ctrl *gomock.Controller, treeID int64) storage.AdminStorage {
	tree := *stestonly.MapTree
	tree.TreeId = treeID

	adminStorage := storage.NewMockAdminStorage(ctrl)
	adminTX := storage.NewMockReadOnlyAdminTX(ctrl)

	adminStorage.EXPECT().Snapshot(gomock.Any()).MaxTimes(1).Return(adminTX, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), treeID).MaxTimes(1).Return(&tree, nil)
	adminTX.EXPECT().Close().MaxTimes(1).Return(nil)
	adminTX.EXPECT().Commit().MaxTimes(1).Return(nil)

	return adminStorage
}
