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

package quotaapi

import (
	"testing"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian/quota/etcd/quotapb"
	"github.com/google/trillian/quota/etcd/storagepb"
	"google.golang.org/genproto/protobuf/field_mask"
)

var (
	apiSequencingConfig = &quotapb.Config{
		Name:      "quotas/global/write/config",
		State:     quotapb.Config_ENABLED,
		MaxTokens: 100,
		ReplenishmentStrategy: &quotapb.Config_SequencingBased{
			SequencingBased: &quotapb.SequencingBasedStrategy{},
		},
	}
	apiTimeConfig = &quotapb.Config{
		Name:      "quotas/users/llama/write/config",
		State:     quotapb.Config_DISABLED,
		MaxTokens: 200,
		ReplenishmentStrategy: &quotapb.Config_TimeBased{
			TimeBased: &quotapb.TimeBasedStrategy{
				TokensToReplenish:        10,
				ReplenishIntervalSeconds: 30,
			},
		},
	}
	storageSequencingConfig = &storagepb.Config{
		Name:      apiSequencingConfig.Name,
		State:     storagepb.Config_ENABLED,
		MaxTokens: apiSequencingConfig.MaxTokens,
		ReplenishmentStrategy: &storagepb.Config_SequencingBased{
			SequencingBased: &storagepb.SequencingBasedStrategy{},
		},
	}
	storageTimeConfig = &storagepb.Config{
		Name:      apiTimeConfig.Name,
		State:     storagepb.Config_DISABLED,
		MaxTokens: apiTimeConfig.MaxTokens,
		ReplenishmentStrategy: &storagepb.Config_TimeBased{
			TimeBased: &storagepb.TimeBasedStrategy{
				TokensToReplenish:        apiTimeConfig.GetTimeBased().TokensToReplenish,
				ReplenishIntervalSeconds: apiTimeConfig.GetTimeBased().ReplenishIntervalSeconds,
			},
		},
	}
)

func TestValidateMask(t *testing.T) {
	tests := []struct {
		desc    string
		mask    *field_mask.FieldMask
		wantErr bool
	}{
		{
			desc: "commonFields",
			mask: commonMask,
		},
		{
			desc: "sequencingBased",
			mask: sequencingBasedMask,
		},
		{
			desc: "timeBased",
			mask: timeBasedMask,
		},
		{
			desc:    "sequencingAndTime",
			mask:    &field_mask.FieldMask{Paths: []string{sequencingBasedPath, timeBasedPath}},
			wantErr: true,
		},
		{
			desc:    "timeAndSequencing",
			mask:    &field_mask.FieldMask{Paths: []string{timeBasedPath, sequencingBasedPath}},
			wantErr: true,
		},
		{
			desc:    "unknownPaths",
			mask:    &field_mask.FieldMask{Paths: []string{statePath, "NOT_A_FIELD", maxTokensPath}},
			wantErr: true,
		},
		{
			desc:    "namePath", // readonly
			mask:    &field_mask.FieldMask{Paths: []string{"name"}},
			wantErr: true,
		},
	}
	for _, test := range tests {
		err := validateMask(test.mask)
		if gotErr := err != nil; gotErr != test.wantErr {
			t.Errorf("%v: validateMask() returned err = %v, wantErr = %v", test.desc, err, test.wantErr)
		}
	}
}

func TestApplyMask(t *testing.T) {
	// destSequencingConfig must match apiSequencingConfig after the test
	// name is manually copied, as it's a readonly field.
	destSequencingConfig := proto.Clone(storageTimeConfig).(*storagepb.Config)
	destSequencingConfig.Name = apiSequencingConfig.Name

	// destTimeConfig must match apiTimeConfig after the test
	destTimeConfig := proto.Clone(storageSequencingConfig).(*storagepb.Config)
	destTimeConfig.Name = apiTimeConfig.Name

	destClearSequencing := proto.Clone(storageSequencingConfig).(*storagepb.Config)
	wantClearSequencing := destClearSequencing
	wantClearSequencing.ReplenishmentStrategy = nil

	destClearTime := proto.Clone(storageTimeConfig).(*storagepb.Config)
	wantClearTime := destClearTime
	wantClearTime.ReplenishmentStrategy = nil

	tests := []struct {
		desc       string
		src        *quotapb.Config
		dest, want *storagepb.Config
		mask       *field_mask.FieldMask
	}{
		{
			desc: "applyToBlank",
			src:  apiTimeConfig,
			dest: &storagepb.Config{},
			mask: &field_mask.FieldMask{Paths: []string{statePath, maxTokensPath}},
			want: &storagepb.Config{
				State:     storagepb.Config_DISABLED,
				MaxTokens: apiTimeConfig.MaxTokens,
			},
		},
		{
			desc: "sequencingBasedOverwrite",
			src:  apiSequencingConfig,
			dest: destSequencingConfig,
			mask: sequencingBasedMask,
			want: storageSequencingConfig,
		},
		{
			desc: "timeBasedOverwrite",
			src:  apiTimeConfig,
			dest: destTimeConfig,
			mask: timeBasedMask,
			want: storageTimeConfig,
		},
		{
			desc: "clearSequencingIfNil",
			src:  &quotapb.Config{},
			dest: destClearSequencing,
			mask: &field_mask.FieldMask{Paths: []string{sequencingBasedPath}},
			want: wantClearSequencing,
		},
		{
			desc: "clearTimeIfNil",
			src:  &quotapb.Config{},
			dest: destClearTime,
			mask: &field_mask.FieldMask{Paths: []string{timeBasedPath}},
			want: wantClearTime,
		},
	}
	for _, test := range tests {
		applyMask(test.src, test.dest, test.mask)
		if !proto.Equal(test.dest, test.want) {
			t.Errorf("%v: post-applyMask() diff (-got +want):\n%v", test.desc, cmp.Diff(test.dest, test.want))
		}
	}
}

func TestConvert_APIAndStorage(t *testing.T) {
	tests := []struct {
		desc    string
		api     *quotapb.Config
		storage *storagepb.Config
	}{
		{
			desc:    "sequencingBased",
			api:     apiSequencingConfig,
			storage: storageSequencingConfig,
		},
		{
			desc:    "timeBased",
			api:     apiTimeConfig,
			storage: storageTimeConfig,
		},
		{
			desc:    "zeroed",
			api:     &quotapb.Config{},
			storage: &storagepb.Config{},
		},
	}
	for _, test := range tests {
		if got, want := convertToAPI(test.storage), test.api; !proto.Equal(got, want) {
			t.Errorf("%v: post-convertToAPI() diff (-got +want):\n%v", test.desc, cmp.Diff(got, want))
		}
		if got, want := convertToStorage(test.api), test.storage; !proto.Equal(got, want) {
			t.Errorf("%v: post-convertToStorage() diff (-got +want):\n%v", test.desc, cmp.Diff(got, want))
		}
	}
}
