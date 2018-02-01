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
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/kylelemons/godebug/pretty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	mapID1 = int64(1)
)

var (
	signedMapRootID1Rev0 = trillian.SignedMapRoot{
		TimestampNanos: 1508235889834964600,
		RootHash:       []byte("\306h\237\020\201*\t\200\227m\2253\3308u(!f\025\225g\3545\025W\026\301A:\365=j"),
		Signature: &sigpb.DigitallySigned{
			HashAlgorithm:      sigpb.DigitallySigned_SHA256,
			SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
			Signature:          []byte("0F\002!\000\307b\255\223\353\23615&\022\263\323\341\342+\276\274$\rX?\366\014U\362\006\376\0269rcm\002!\000\241*\255\220\301\263D\033\275\374\340A\377\337\354\202\331%au\3179\000O\r9\237\302\021\r\363\263"),
		},
		MapId:       mapID1,
		MapRevision: 0,
	}

	signedMapRootID1Rev1 = trillian.SignedMapRoot{
		TimestampNanos: 1508235889834964600,
		RootHash:       []byte("\306h\237\020\201*\t\200\227m\2253\3308u(!f\025\225g\3545\025W\026\301A:\365=j"),
		Signature: &sigpb.DigitallySigned{
			HashAlgorithm:      sigpb.DigitallySigned_SHA256,
			SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
			Signature:          []byte("0F\002!\000\307b\255\223\353\23615&\022\263\323\341\342+\276\274$\rX?\366\014U\362\006\376\0269rcm\002!\000\241*\255\220\301\263D\033\275\374\340A\377\337\354\202\331%au\3179\000O\r9\237\302\021\r\363\263"),
		},
		MapId:       mapID1,
		MapRevision: 1,
	}
)

func TestIsHealthy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		desc          string
		accessibleErr error
	}{
		{"healthy", nil},
		{"unhealthy", errors.New("DB not happy")},
	}

	for _, test := range tests {
		mockStorage := storage.NewMockMapStorage(ctrl)
		mockStorage.EXPECT().CheckDatabaseAccessible(gomock.Any()).Return(test.accessibleErr)

		server := NewTrillianMapServer(extension.Registry{
			AdminStorage: mockAdminStorageForMap(ctrl, 1, mapID1),
			MapStorage:   mockStorage,
		})

		wantErr := test.accessibleErr != nil
		err := server.IsHealthy()
		if gotErr := err != nil; gotErr != wantErr {
			t.Errorf("%s: IsHealthy() err? %t want? %t (err=%v)", test.desc, gotErr, wantErr, err)
		}
	}
}

func TestInitMap(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		desc     string
		txErr    error
		wantInit bool
		root     []byte
		wantCode codes.Code
	}{
		{desc: "init new log", txErr: storage.ErrTreeNeedsInit, wantInit: true, root: nil, wantCode: codes.OK},
		{desc: "init new log, no err", txErr: nil, wantInit: true, root: nil, wantCode: codes.OK},
		{desc: "init already initialised log", txErr: nil, wantInit: false, root: []byte{}, wantCode: codes.AlreadyExists},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := storage.NewMockMapStorage(ctrl)
			mockTx := storage.NewMockMapTreeTX(ctrl)
			mockStorage.EXPECT().ReadWriteTransaction(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(stestonly.RunOnMapTX(mockTX))

			mockTx.EXPECT().IsOpen().AnyTimes().Return(false)
			mockTx.EXPECT().Close().Return(nil)
			mockTx.EXPECT().LatestSignedMapRoot(gomock.Any()).Return(
				trillian.SignedMapRoot{RootHash: tc.root}, nil)
			if tc.wantInit {
				mockTx.EXPECT().Commit().Return(nil)
				mockTx.EXPECT().StoreSignedMapRoot(gomock.Any(), gomock.Any())
			}

			server := NewTrillianMapServer(extension.Registry{
				AdminStorage: mockAdminStorageForMap(ctrl, 2, mapID1),
				MapStorage:   mockStorage,
			})

			c, err := server.InitMap(ctx, &trillian.InitMapRequest{
				MapId: mapID1,
			})
			if got, want := status.Code(err), tc.wantCode; got != want {
				t.Errorf("InitMap returned %v, want %v", got, want)
			}
			if tc.wantInit {
				if err != nil {
					t.Fatalf("InitLog returned %v, want no error", err)
				}
				if c.Created == nil {
					t.Error("InitLog first attempt didn't return the created STH.")
				}
			} else {
				if err == nil {
					t.Errorf("InitLog returned nil, want error")
				}
			}
		})
	}
}

func TestGetSignedMapRoot_NotInitialised(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	mockStorage := storage.NewMockMapStorage(ctrl)
	mockTX := storage.NewMockMapTreeTX(ctrl)
	server := NewTrillianMapServer(extension.Registry{
		MapStorage: mockStorage,
	})
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), gomock.Any()).Return(mockTX, nil)
	mockTX.EXPECT().LatestSignedMapRoot(gomock.Any()).Return(trillian.SignedMapRoot{}, storage.ErrTreeNeedsInit)
	mockTX.EXPECT().Close()

	smrResp, err := server.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{})

	if err == nil {
		t.Errorf("GetSignedMapRoot()=_, nil want err")
	}
	if smrResp != nil {
		t.Errorf("GetSignedMapRoot()=%v, _ want nil", smrResp)
	}
}

func TestGetSignedMapRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	tests := []struct {
		desc               string
		req                *trillian.GetSignedMapRootRequest
		mapRoot            trillian.SignedMapRoot
		snapShErr, lsmrErr error
	}{
		{
			desc:    "Map is empty, head at revision 0",
			req:     &trillian.GetSignedMapRootRequest{MapId: mapID1},
			mapRoot: signedMapRootID1Rev0,
		},
		{
			desc:    "Map has leaves, head > revision 0",
			req:     &trillian.GetSignedMapRootRequest{MapId: mapID1},
			mapRoot: signedMapRootID1Rev1,
		},
		{
			desc:    "LatestSignedMapRoot returns error",
			req:     &trillian.GetSignedMapRootRequest{MapId: mapID1},
			lsmrErr: errors.New("sql: no rows in result set"),
		},
		{
			desc:      "Snapshot returns Error",
			req:       &trillian.GetSignedMapRootRequest{MapId: mapID1},
			snapShErr: errors.New("unknown map"),
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			adminStorage := mockAdminStorageForMap(ctrl, 2, mapID1)
			mockStorage := storage.NewMockMapStorage(ctrl)
			mockTx := storage.NewMockMapTreeTX(ctrl)

			// Calls from GetSignedMapRoot()
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
				AdminStorage: adminStorage,
				MapStorage:   mockStorage,
			})

			smrResp, err := server.GetSignedMapRoot(ctx, test.req)

			wantErr := test.snapShErr != nil || test.lsmrErr != nil
			if gotErr := err != nil; gotErr != wantErr {
				t.Errorf("GetSignedMapRoot()=_, err? %t want? %t (err=%v)", gotErr, wantErr, err)
			}
			if err != nil {
				return
			}
			want := &trillian.GetSignedMapRootResponse{MapRoot: &test.mapRoot}
			if got := smrResp; !proto.Equal(got, want) {
				diff := pretty.Compare(got, want)
				t.Errorf("GetSignedMapRoot() got != want, diff:\n%v", diff)
			}
		})
	}
}

func TestGetSignedMapRootByRevision_NotInitialised(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	mockStorage := storage.NewMockMapStorage(ctrl)
	mockTX := storage.NewMockMapTreeTX(ctrl)
	server := NewTrillianMapServer(extension.Registry{
		MapStorage: mockStorage,
	})
	mockStorage.EXPECT().SnapshotForTree(gomock.Any(), gomock.Any()).Return(mockTX, nil)
	mockTX.EXPECT().GetSignedMapRoot(gomock.Any(), gomock.Any()).Return(trillian.SignedMapRoot{}, storage.ErrTreeNeedsInit)
	mockTX.EXPECT().Close()

	smrResp, err := server.GetSignedMapRootByRevision(ctx, &trillian.GetSignedMapRootByRevisionRequest{
		Revision: 1,
	})

	if err == nil {
		t.Errorf("GetSignedMapRootByRevision()=_, nil want err? true")
	}
	if smrResp != nil {
		t.Errorf("GetSignedMapRootByRevision()=%v, _ want nil", smrResp)
	}
}

func TestGetSignedMapRootByRevision(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := context.Background()

	tests := []struct {
		desc               string
		req                *trillian.GetSignedMapRootByRevisionRequest
		mapRoot            trillian.SignedMapRoot
		snapShErr, lsmrErr error
		wantErr            bool
	}{
		{
			desc:    "Request revision 0 for empty map",
			req:     &trillian.GetSignedMapRootByRevisionRequest{MapId: mapID1},
			lsmrErr: errors.New("sql: no rows in result set"),
			wantErr: true,
		},
		{
			desc:    "Request invalid -ve revision",
			req:     &trillian.GetSignedMapRootByRevisionRequest{MapId: mapID1, Revision: -1},
			wantErr: true,
		},
		{
			desc:    "Request future revision (123) for empty map",
			req:     &trillian.GetSignedMapRootByRevisionRequest{MapId: mapID1, Revision: 123},
			lsmrErr: errors.New("sql: no rows in result set"),
			wantErr: true,
		},
		{
			desc:    "Request revision >0 for non-empty map",
			req:     &trillian.GetSignedMapRootByRevisionRequest{MapId: mapID1, Revision: 1},
			mapRoot: signedMapRootID1Rev1,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			adminStorage := mockAdminStorageForMap(ctrl, 2, mapID1)
			mockStorage := storage.NewMockMapStorage(ctrl)
			mockTx := storage.NewMockMapTreeTX(ctrl)

			if !test.wantErr || !(test.lsmrErr == nil && test.snapShErr == nil) {
				mockStorage.EXPECT().SnapshotForTree(gomock.Any(), test.req.MapId).Return(mockTx, test.snapShErr)
				if test.snapShErr == nil {
					mockTx.EXPECT().GetSignedMapRoot(gomock.Any(), test.req.Revision).Return(test.mapRoot, test.lsmrErr)
					if test.lsmrErr == nil {
						mockTx.EXPECT().Commit().Return(nil)
					}
					mockTx.EXPECT().Close().Return(nil)
					mockTx.EXPECT().IsOpen().AnyTimes().Return(false)
				}
			}

			server := NewTrillianMapServer(extension.Registry{
				AdminStorage: adminStorage,
				MapStorage:   mockStorage,
			})

			smrResp, err := server.GetSignedMapRootByRevision(ctx, test.req)

			if gotErr := err != nil; gotErr != test.wantErr {
				t.Errorf("GetSignedMapRootByRevision()=_, err? %t want? %t (err=%v)", gotErr, test.wantErr, err)
			}
			if err != nil {
				return
			}
			want := &trillian.GetSignedMapRootResponse{MapRoot: &test.mapRoot}
			if got := smrResp; !proto.Equal(got, want) {
				diff := pretty.Compare(got, want)
				t.Errorf("GetSignedMapRootByRevision() got != want, diff:\n%v", diff)
			}
		})
	}
}

func mockAdminStorageForMap(ctrl *gomock.Controller, times int, treeID int64) storage.AdminStorage {
	tree := *stestonly.MapTree
	tree.TreeId = treeID

	adminStorage := storage.NewMockAdminStorage(ctrl)
	adminTX := storage.NewMockReadOnlyAdminTX(ctrl)

	adminStorage.EXPECT().Snapshot(gomock.Any()).MaxTimes(times).Return(adminTX, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), treeID).MaxTimes(times).Return(&tree, nil)
	adminTX.EXPECT().Close().MaxTimes(times).Return(nil)
	adminTX.EXPECT().Commit().MaxTimes(times).Return(nil)

	return adminStorage
}
