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
	"fmt"

	"github.com/google/trillian/quota/etcd/quotapb"
	"github.com/google/trillian/quota/etcd/storagepb"
	"google.golang.org/genproto/protobuf/field_mask"
)

const (
	statePath           = "state"
	maxTokensPath       = "max_tokens"
	sequencingBasedPath = "sequencing_based"
	timeBasedPath       = "time_based"
)

var (
	commonMask          = &field_mask.FieldMask{Paths: []string{statePath, maxTokensPath}}
	sequencingBasedMask = &field_mask.FieldMask{Paths: append(commonMask.Paths, sequencingBasedPath)}
	timeBasedMask       = &field_mask.FieldMask{Paths: append(commonMask.Paths, timeBasedPath)}

	errBothReplenishmentStrategies = fmt.Errorf("both %q and %q paths specified, please pick a single replenishment strategy", sequencingBasedPath, timeBasedPath)
)

// validateMask returns nil if mask is a valid Config mask, error otherwise.
// Paths must match the proto name of the fields (e.g., "time_based", not "TimeBased").
// If fields belong to a oneof (such as "sequencing_based" and "time_based"), then only one field of
// the oneof may be specified.
func validateMask(mask *field_mask.FieldMask) error {
	sequencingBasedFound := false
	timeBasedFound := false
	for _, path := range mask.Paths {
		switch path {
		case statePath, maxTokensPath:
			// OK
		case sequencingBasedPath:
			if timeBasedFound {
				return errBothReplenishmentStrategies
			}
			sequencingBasedFound = true
		case timeBasedPath:
			if sequencingBasedFound {
				return errBothReplenishmentStrategies
			}
			timeBasedFound = true
		default:
			return fmt.Errorf("invalid field path for Config: %q", path)
		}
	}
	return nil
}

// applyMask copies the fields specified by mask from src to dest. The mask must be first validated
// by validateMask(), as unknown paths are simply ignored by applyMask.
// Paths must match the proto name of the fields (e.g., "time_based", not "TimeBased").
func applyMask(src *quotapb.Config, dest *storagepb.Config, mask *field_mask.FieldMask) {
	for _, path := range mask.Paths {
		switch path {
		case statePath:
			dest.State = storagepb.Config_State(storagepb.Config_State_value[src.State.String()])
		case maxTokensPath:
			dest.MaxTokens = src.MaxTokens
		case sequencingBasedPath:
			if src.GetSequencingBased() == nil {
				dest.ReplenishmentStrategy = nil
			} else {
				dest.ReplenishmentStrategy = &storagepb.Config_SequencingBased{
					SequencingBased: &storagepb.SequencingBasedStrategy{},
				}
			}
		case timeBasedPath:
			if tb := src.GetTimeBased(); tb == nil {
				dest.ReplenishmentStrategy = nil
			} else {
				dest.ReplenishmentStrategy = &storagepb.Config_TimeBased{
					TimeBased: &storagepb.TimeBasedStrategy{
						TokensToReplenish:        tb.GetTokensToReplenish(),
						ReplenishIntervalSeconds: tb.GetReplenishIntervalSeconds(),
					},
				}
			}
		}
	}
}

// convertToAPI returns the API representation of a storagepb.Config proto.
func convertToAPI(src *storagepb.Config) *quotapb.Config {
	dest := &quotapb.Config{
		Name:      src.Name,
		State:     quotapb.Config_State(quotapb.Config_State_value[src.State.String()]),
		MaxTokens: src.MaxTokens,
	}
	sb := src.GetSequencingBased()
	tb := src.GetTimeBased()
	switch {
	case sb != nil:
		dest.ReplenishmentStrategy = &quotapb.Config_SequencingBased{
			SequencingBased: &quotapb.SequencingBasedStrategy{},
		}
	case tb != nil:
		dest.ReplenishmentStrategy = &quotapb.Config_TimeBased{
			TimeBased: &quotapb.TimeBasedStrategy{
				TokensToReplenish:        tb.TokensToReplenish,
				ReplenishIntervalSeconds: tb.ReplenishIntervalSeconds,
			},
		}
	}
	return dest
}

// convertToStorage returns the storage representation of a quotapb.Config proto.
func convertToStorage(src *quotapb.Config) *storagepb.Config {
	// Instead of potentially duplicating logic, let's take advantage of applyMask by picking a
	// pre-made mask that contains all fields we care about and apply it to a new proto.
	var mask *field_mask.FieldMask
	switch {
	case src.GetSequencingBased() != nil:
		mask = sequencingBasedMask
	case src.GetTimeBased() != nil:
		mask = timeBasedMask
	default:
		mask = commonMask
	}
	dest := &storagepb.Config{Name: src.Name}
	applyMask(src, dest, mask)
	return dest
}
