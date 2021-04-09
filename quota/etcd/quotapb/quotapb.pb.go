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

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.26.0
// 	protoc        v3.12.4
// source: quotapb.proto

package quotapb

import (
	empty "github.com/golang/protobuf/ptypes/empty"
	_ "google.golang.org/genproto/googleapis/api/annotations"
	field_mask "google.golang.org/genproto/protobuf/field_mask"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Possible states of a quota configuration.
type Config_State int32

const (
	// Unknown quota state. Invalid.
	Config_UNKNOWN_CONFIG_STATE Config_State = 0
	// Quota is enabled.
	Config_ENABLED Config_State = 1
	// Quota is disabled (considered infinite).
	Config_DISABLED Config_State = 2
)

// Enum value maps for Config_State.
var (
	Config_State_name = map[int32]string{
		0: "UNKNOWN_CONFIG_STATE",
		1: "ENABLED",
		2: "DISABLED",
	}
	Config_State_value = map[string]int32{
		"UNKNOWN_CONFIG_STATE": 0,
		"ENABLED":              1,
		"DISABLED":             2,
	}
)

func (x Config_State) Enum() *Config_State {
	p := new(Config_State)
	*p = x
	return p
}

func (x Config_State) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Config_State) Descriptor() protoreflect.EnumDescriptor {
	return file_quotapb_proto_enumTypes[0].Descriptor()
}

func (Config_State) Type() protoreflect.EnumType {
	return &file_quotapb_proto_enumTypes[0]
}

func (x Config_State) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Config_State.Descriptor instead.
func (Config_State) EnumDescriptor() ([]byte, []int) {
	return file_quotapb_proto_rawDescGZIP(), []int{0, 0}
}

// Possible views for ListConfig.
type ListConfigsRequest_ListView int32

const (
	// Only the Config name gets returned.
	ListConfigsRequest_BASIC ListConfigsRequest_ListView = 0
	// Complete Config.
	ListConfigsRequest_FULL ListConfigsRequest_ListView = 1
)

// Enum value maps for ListConfigsRequest_ListView.
var (
	ListConfigsRequest_ListView_name = map[int32]string{
		0: "BASIC",
		1: "FULL",
	}
	ListConfigsRequest_ListView_value = map[string]int32{
		"BASIC": 0,
		"FULL":  1,
	}
)

func (x ListConfigsRequest_ListView) Enum() *ListConfigsRequest_ListView {
	p := new(ListConfigsRequest_ListView)
	*p = x
	return p
}

func (x ListConfigsRequest_ListView) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (ListConfigsRequest_ListView) Descriptor() protoreflect.EnumDescriptor {
	return file_quotapb_proto_enumTypes[1].Descriptor()
}

func (ListConfigsRequest_ListView) Type() protoreflect.EnumType {
	return &file_quotapb_proto_enumTypes[1]
}

func (x ListConfigsRequest_ListView) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use ListConfigsRequest_ListView.Descriptor instead.
func (ListConfigsRequest_ListView) EnumDescriptor() ([]byte, []int) {
	return file_quotapb_proto_rawDescGZIP(), []int{6, 0}
}

// Configuration of a quota.
//
// Quotas contain a certain number of tokens that get applied to their
// corresponding entities. Global quotas apply to all operations, tree and user
// quotas to certain trees and users, respectively.
//
// Performing an operation costs a certain number of tokens (usually one). Once
// a quota has no more tokens available, requests that would subtract from it
// are denied with a resource_exhausted error.
//
// Tokens may be replenished in two different ways: either by passage of time or
// sequencing progress. Time-based replenishment adds a fixed amount of tokens
// after a certain interval. Sequencing-based adds a token for each leaf
// processed by the sequencer. Sequencing-based replenishment may only be used
// with global and tree quotas.
//
// A quota may be disabled or removed at any time. The effect is the same: a
// disabled or non-existing quota is considered infinite by the quota system.
// (Disabling is handy if you plan to re-enable a quota later on.)
type Config struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Name of the config, eg, “quotas/trees/1234/read/config”.
	// Readonly.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// State of the config.
	State Config_State `protobuf:"varint,2,opt,name=state,proto3,enum=quotapb.Config_State" json:"state,omitempty"`
	// Max number of tokens available for the config.
	MaxTokens int64 `protobuf:"varint,3,opt,name=max_tokens,json=maxTokens,proto3" json:"max_tokens,omitempty"`
	// Replenishment strategy used by the config.
	//
	// Types that are assignable to ReplenishmentStrategy:
	//	*Config_SequencingBased
	//	*Config_TimeBased
	ReplenishmentStrategy isConfig_ReplenishmentStrategy `protobuf_oneof:"replenishment_strategy"`
	// Current number of tokens available for the config.
	// May be higher than max_tokens for DISABLED configs, which are considered to
	// have "infinite" tokens.
	// Readonly.
	CurrentTokens int64 `protobuf:"varint,6,opt,name=current_tokens,json=currentTokens,proto3" json:"current_tokens,omitempty"`
}

func (x *Config) Reset() {
	*x = Config{}
	if protoimpl.UnsafeEnabled {
		mi := &file_quotapb_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Config) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Config) ProtoMessage() {}

func (x *Config) ProtoReflect() protoreflect.Message {
	mi := &file_quotapb_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Config.ProtoReflect.Descriptor instead.
func (*Config) Descriptor() ([]byte, []int) {
	return file_quotapb_proto_rawDescGZIP(), []int{0}
}

func (x *Config) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Config) GetState() Config_State {
	if x != nil {
		return x.State
	}
	return Config_UNKNOWN_CONFIG_STATE
}

func (x *Config) GetMaxTokens() int64 {
	if x != nil {
		return x.MaxTokens
	}
	return 0
}

func (m *Config) GetReplenishmentStrategy() isConfig_ReplenishmentStrategy {
	if m != nil {
		return m.ReplenishmentStrategy
	}
	return nil
}

func (x *Config) GetSequencingBased() *SequencingBasedStrategy {
	if x, ok := x.GetReplenishmentStrategy().(*Config_SequencingBased); ok {
		return x.SequencingBased
	}
	return nil
}

func (x *Config) GetTimeBased() *TimeBasedStrategy {
	if x, ok := x.GetReplenishmentStrategy().(*Config_TimeBased); ok {
		return x.TimeBased
	}
	return nil
}

func (x *Config) GetCurrentTokens() int64 {
	if x != nil {
		return x.CurrentTokens
	}
	return 0
}

type isConfig_ReplenishmentStrategy interface {
	isConfig_ReplenishmentStrategy()
}

type Config_SequencingBased struct {
	// Sequencing-based replenishment settings.
	SequencingBased *SequencingBasedStrategy `protobuf:"bytes,4,opt,name=sequencing_based,json=sequencingBased,proto3,oneof"`
}

type Config_TimeBased struct {
	// Time-based replenishment settings.
	TimeBased *TimeBasedStrategy `protobuf:"bytes,5,opt,name=time_based,json=timeBased,proto3,oneof"`
}

func (*Config_SequencingBased) isConfig_ReplenishmentStrategy() {}

func (*Config_TimeBased) isConfig_ReplenishmentStrategy() {}

// Sequencing-based replenishment strategy settings.
//
// Only global/write and trees/write quotas may use sequencing-based
// replenishment.
type SequencingBasedStrategy struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *SequencingBasedStrategy) Reset() {
	*x = SequencingBasedStrategy{}
	if protoimpl.UnsafeEnabled {
		mi := &file_quotapb_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SequencingBasedStrategy) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SequencingBasedStrategy) ProtoMessage() {}

func (x *SequencingBasedStrategy) ProtoReflect() protoreflect.Message {
	mi := &file_quotapb_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SequencingBasedStrategy.ProtoReflect.Descriptor instead.
func (*SequencingBasedStrategy) Descriptor() ([]byte, []int) {
	return file_quotapb_proto_rawDescGZIP(), []int{1}
}

// Time-based replenishment strategy settings.
type TimeBasedStrategy struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Number of tokens to replenish at every replenish_interval_seconds.
	TokensToReplenish int64 `protobuf:"varint,1,opt,name=tokens_to_replenish,json=tokensToReplenish,proto3" json:"tokens_to_replenish,omitempty"`
	// Interval at which tokens_to_replenish get replenished.
	ReplenishIntervalSeconds int64 `protobuf:"varint,2,opt,name=replenish_interval_seconds,json=replenishIntervalSeconds,proto3" json:"replenish_interval_seconds,omitempty"`
}

func (x *TimeBasedStrategy) Reset() {
	*x = TimeBasedStrategy{}
	if protoimpl.UnsafeEnabled {
		mi := &file_quotapb_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TimeBasedStrategy) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TimeBasedStrategy) ProtoMessage() {}

func (x *TimeBasedStrategy) ProtoReflect() protoreflect.Message {
	mi := &file_quotapb_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TimeBasedStrategy.ProtoReflect.Descriptor instead.
func (*TimeBasedStrategy) Descriptor() ([]byte, []int) {
	return file_quotapb_proto_rawDescGZIP(), []int{2}
}

func (x *TimeBasedStrategy) GetTokensToReplenish() int64 {
	if x != nil {
		return x.TokensToReplenish
	}
	return 0
}

func (x *TimeBasedStrategy) GetReplenishIntervalSeconds() int64 {
	if x != nil {
		return x.ReplenishIntervalSeconds
	}
	return 0
}

// CreateConfig request.
type CreateConfigRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Name of the config to create.
	// For example, "quotas/global/read/config" (global/read quota) or
	// "quotas/trees/1234/write/config" (write quota for tree 1234).
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Config to be created.
	Config *Config `protobuf:"bytes,2,opt,name=config,proto3" json:"config,omitempty"`
}

func (x *CreateConfigRequest) Reset() {
	*x = CreateConfigRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_quotapb_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CreateConfigRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CreateConfigRequest) ProtoMessage() {}

func (x *CreateConfigRequest) ProtoReflect() protoreflect.Message {
	mi := &file_quotapb_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CreateConfigRequest.ProtoReflect.Descriptor instead.
func (*CreateConfigRequest) Descriptor() ([]byte, []int) {
	return file_quotapb_proto_rawDescGZIP(), []int{3}
}

func (x *CreateConfigRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *CreateConfigRequest) GetConfig() *Config {
	if x != nil {
		return x.Config
	}
	return nil
}

// DeleteConfig request.
type DeleteConfigRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Name of the config to delete.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
}

func (x *DeleteConfigRequest) Reset() {
	*x = DeleteConfigRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_quotapb_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *DeleteConfigRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DeleteConfigRequest) ProtoMessage() {}

func (x *DeleteConfigRequest) ProtoReflect() protoreflect.Message {
	mi := &file_quotapb_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DeleteConfigRequest.ProtoReflect.Descriptor instead.
func (*DeleteConfigRequest) Descriptor() ([]byte, []int) {
	return file_quotapb_proto_rawDescGZIP(), []int{4}
}

func (x *DeleteConfigRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

// GetConfig request.
type GetConfigRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Name of the config to retrieve.
	// For example, "quotas/global/read/config".
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
}

func (x *GetConfigRequest) Reset() {
	*x = GetConfigRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_quotapb_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetConfigRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetConfigRequest) ProtoMessage() {}

func (x *GetConfigRequest) ProtoReflect() protoreflect.Message {
	mi := &file_quotapb_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetConfigRequest.ProtoReflect.Descriptor instead.
func (*GetConfigRequest) Descriptor() ([]byte, []int) {
	return file_quotapb_proto_rawDescGZIP(), []int{5}
}

func (x *GetConfigRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

// ListConfig request.
type ListConfigsRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Names of the config to retrieve. For example, "quotas/global/read/config".
	// If empty, all configs are listed.
	// Name components may be substituted by "-" to search for all variations of
	// that component. For example:
	// - "quotas/global/-/config" (both read and write global quotas)
	// - "quotas/trees/-/-/config" (all tree quotas)
	Names []string `protobuf:"bytes,1,rep,name=names,proto3" json:"names,omitempty"`
	// View specifies how much data to return.
	View ListConfigsRequest_ListView `protobuf:"varint,2,opt,name=view,proto3,enum=quotapb.ListConfigsRequest_ListView" json:"view,omitempty"`
}

func (x *ListConfigsRequest) Reset() {
	*x = ListConfigsRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_quotapb_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ListConfigsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListConfigsRequest) ProtoMessage() {}

func (x *ListConfigsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_quotapb_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListConfigsRequest.ProtoReflect.Descriptor instead.
func (*ListConfigsRequest) Descriptor() ([]byte, []int) {
	return file_quotapb_proto_rawDescGZIP(), []int{6}
}

func (x *ListConfigsRequest) GetNames() []string {
	if x != nil {
		return x.Names
	}
	return nil
}

func (x *ListConfigsRequest) GetView() ListConfigsRequest_ListView {
	if x != nil {
		return x.View
	}
	return ListConfigsRequest_BASIC
}

// ListConfig response.
type ListConfigsResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Configs matching the request filter.
	Configs []*Config `protobuf:"bytes,1,rep,name=configs,proto3" json:"configs,omitempty"`
}

func (x *ListConfigsResponse) Reset() {
	*x = ListConfigsResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_quotapb_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ListConfigsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListConfigsResponse) ProtoMessage() {}

func (x *ListConfigsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_quotapb_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListConfigsResponse.ProtoReflect.Descriptor instead.
func (*ListConfigsResponse) Descriptor() ([]byte, []int) {
	return file_quotapb_proto_rawDescGZIP(), []int{7}
}

func (x *ListConfigsResponse) GetConfigs() []*Config {
	if x != nil {
		return x.Configs
	}
	return nil
}

// Updates a quota config according to the update_mask provided.
//
// Some config changes will cause the current number of tokens to be updated, as
// listed below:
//
// * If max_tokens is reduced and the current number of tokens is greater than
//   the new max_tokens, the current number of tokens is reduced to max_tokens.
//   This happens so the quota is immediately conformant to the new
//   configuration.
//
// * A state transition from disabled to enabled causes the quota to be fully
//   replenished. This happens so the re-enabled quota will enter in action in a
//   known, predictable state.
//
// A "full replenish", also called "reset", may be forced via the reset_quota
// parameter, regardless of any other changes. For convenience, reset only
// requests (name and reset_quota = true) are allowed.
type UpdateConfigRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Name of the config to update.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Config to update. Only the fields specified by update_mask need to be
	// filled.
	Config *Config `protobuf:"bytes,2,opt,name=config,proto3" json:"config,omitempty"`
	// Fields modified by the update request.
	// For example: "state" or "max_tokens".
	UpdateMask *field_mask.FieldMask `protobuf:"bytes,3,opt,name=update_mask,json=updateMask,proto3" json:"update_mask,omitempty"`
	// If true the updated quota is reset, regardless of the update's contents.
	// A reset quota is replenished to its maximum number of tokens.
	ResetQuota bool `protobuf:"varint,4,opt,name=reset_quota,json=resetQuota,proto3" json:"reset_quota,omitempty"`
}

func (x *UpdateConfigRequest) Reset() {
	*x = UpdateConfigRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_quotapb_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *UpdateConfigRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UpdateConfigRequest) ProtoMessage() {}

func (x *UpdateConfigRequest) ProtoReflect() protoreflect.Message {
	mi := &file_quotapb_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UpdateConfigRequest.ProtoReflect.Descriptor instead.
func (*UpdateConfigRequest) Descriptor() ([]byte, []int) {
	return file_quotapb_proto_rawDescGZIP(), []int{8}
}

func (x *UpdateConfigRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *UpdateConfigRequest) GetConfig() *Config {
	if x != nil {
		return x.Config
	}
	return nil
}

func (x *UpdateConfigRequest) GetUpdateMask() *field_mask.FieldMask {
	if x != nil {
		return x.UpdateMask
	}
	return nil
}

func (x *UpdateConfigRequest) GetResetQuota() bool {
	if x != nil {
		return x.ResetQuota
	}
	return false
}

var File_quotapb_proto protoreflect.FileDescriptor

var file_quotapb_proto_rawDesc = []byte{
	0x0a, 0x0d, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12,
	0x07, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2f, 0x61, 0x70, 0x69, 0x2f, 0x61, 0x6e, 0x6e, 0x6f, 0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1b, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x1a, 0x20, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2f, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x5f, 0x6d, 0x61, 0x73, 0x6b, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xf3, 0x02, 0x0a, 0x06, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x6e, 0x61, 0x6d, 0x65, 0x12, 0x2b, 0x0a, 0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x0e, 0x32, 0x15, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2e, 0x43, 0x6f,
	0x6e, 0x66, 0x69, 0x67, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x65, 0x52, 0x05, 0x73, 0x74, 0x61, 0x74,
	0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x6d, 0x61, 0x78, 0x5f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x09, 0x6d, 0x61, 0x78, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x73,
	0x12, 0x4d, 0x0a, 0x10, 0x73, 0x65, 0x71, 0x75, 0x65, 0x6e, 0x63, 0x69, 0x6e, 0x67, 0x5f, 0x62,
	0x61, 0x73, 0x65, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x20, 0x2e, 0x71, 0x75, 0x6f,
	0x74, 0x61, 0x70, 0x62, 0x2e, 0x53, 0x65, 0x71, 0x75, 0x65, 0x6e, 0x63, 0x69, 0x6e, 0x67, 0x42,
	0x61, 0x73, 0x65, 0x64, 0x53, 0x74, 0x72, 0x61, 0x74, 0x65, 0x67, 0x79, 0x48, 0x00, 0x52, 0x0f,
	0x73, 0x65, 0x71, 0x75, 0x65, 0x6e, 0x63, 0x69, 0x6e, 0x67, 0x42, 0x61, 0x73, 0x65, 0x64, 0x12,
	0x3b, 0x0a, 0x0a, 0x74, 0x69, 0x6d, 0x65, 0x5f, 0x62, 0x61, 0x73, 0x65, 0x64, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2e, 0x54, 0x69,
	0x6d, 0x65, 0x42, 0x61, 0x73, 0x65, 0x64, 0x53, 0x74, 0x72, 0x61, 0x74, 0x65, 0x67, 0x79, 0x48,
	0x00, 0x52, 0x09, 0x74, 0x69, 0x6d, 0x65, 0x42, 0x61, 0x73, 0x65, 0x64, 0x12, 0x25, 0x0a, 0x0e,
	0x63, 0x75, 0x72, 0x72, 0x65, 0x6e, 0x74, 0x5f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x18, 0x06,
	0x20, 0x01, 0x28, 0x03, 0x52, 0x0d, 0x63, 0x75, 0x72, 0x72, 0x65, 0x6e, 0x74, 0x54, 0x6f, 0x6b,
	0x65, 0x6e, 0x73, 0x22, 0x3c, 0x0a, 0x05, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x18, 0x0a, 0x14,
	0x55, 0x4e, 0x4b, 0x4e, 0x4f, 0x57, 0x4e, 0x5f, 0x43, 0x4f, 0x4e, 0x46, 0x49, 0x47, 0x5f, 0x53,
	0x54, 0x41, 0x54, 0x45, 0x10, 0x00, 0x12, 0x0b, 0x0a, 0x07, 0x45, 0x4e, 0x41, 0x42, 0x4c, 0x45,
	0x44, 0x10, 0x01, 0x12, 0x0c, 0x0a, 0x08, 0x44, 0x49, 0x53, 0x41, 0x42, 0x4c, 0x45, 0x44, 0x10,
	0x02, 0x42, 0x18, 0x0a, 0x16, 0x72, 0x65, 0x70, 0x6c, 0x65, 0x6e, 0x69, 0x73, 0x68, 0x6d, 0x65,
	0x6e, 0x74, 0x5f, 0x73, 0x74, 0x72, 0x61, 0x74, 0x65, 0x67, 0x79, 0x22, 0x19, 0x0a, 0x17, 0x53,
	0x65, 0x71, 0x75, 0x65, 0x6e, 0x63, 0x69, 0x6e, 0x67, 0x42, 0x61, 0x73, 0x65, 0x64, 0x53, 0x74,
	0x72, 0x61, 0x74, 0x65, 0x67, 0x79, 0x22, 0x81, 0x01, 0x0a, 0x11, 0x54, 0x69, 0x6d, 0x65, 0x42,
	0x61, 0x73, 0x65, 0x64, 0x53, 0x74, 0x72, 0x61, 0x74, 0x65, 0x67, 0x79, 0x12, 0x2e, 0x0a, 0x13,
	0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x5f, 0x74, 0x6f, 0x5f, 0x72, 0x65, 0x70, 0x6c, 0x65, 0x6e,
	0x69, 0x73, 0x68, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52, 0x11, 0x74, 0x6f, 0x6b, 0x65, 0x6e,
	0x73, 0x54, 0x6f, 0x52, 0x65, 0x70, 0x6c, 0x65, 0x6e, 0x69, 0x73, 0x68, 0x12, 0x3c, 0x0a, 0x1a,
	0x72, 0x65, 0x70, 0x6c, 0x65, 0x6e, 0x69, 0x73, 0x68, 0x5f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x76,
	0x61, 0x6c, 0x5f, 0x73, 0x65, 0x63, 0x6f, 0x6e, 0x64, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03,
	0x52, 0x18, 0x72, 0x65, 0x70, 0x6c, 0x65, 0x6e, 0x69, 0x73, 0x68, 0x49, 0x6e, 0x74, 0x65, 0x72,
	0x76, 0x61, 0x6c, 0x53, 0x65, 0x63, 0x6f, 0x6e, 0x64, 0x73, 0x22, 0x52, 0x0a, 0x13, 0x43, 0x72,
	0x65, 0x61, 0x74, 0x65, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x27, 0x0a, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2e,
	0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x22, 0x29,
	0x0a, 0x13, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x22, 0x26, 0x0a, 0x10, 0x47, 0x65, 0x74,
	0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a,
	0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d,
	0x65, 0x22, 0x85, 0x01, 0x0a, 0x12, 0x4c, 0x69, 0x73, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x14, 0x0a, 0x05, 0x6e, 0x61, 0x6d, 0x65,
	0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x09, 0x52, 0x05, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x12, 0x38,
	0x0a, 0x04, 0x76, 0x69, 0x65, 0x77, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x24, 0x2e, 0x71,
	0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2e, 0x4c, 0x69, 0x73, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69,
	0x67, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x4c, 0x69, 0x73, 0x74, 0x56, 0x69,
	0x65, 0x77, 0x52, 0x04, 0x76, 0x69, 0x65, 0x77, 0x22, 0x1f, 0x0a, 0x08, 0x4c, 0x69, 0x73, 0x74,
	0x56, 0x69, 0x65, 0x77, 0x12, 0x09, 0x0a, 0x05, 0x42, 0x41, 0x53, 0x49, 0x43, 0x10, 0x00, 0x12,
	0x08, 0x0a, 0x04, 0x46, 0x55, 0x4c, 0x4c, 0x10, 0x01, 0x22, 0x40, 0x0a, 0x13, 0x4c, 0x69, 0x73,
	0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x29, 0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x0f, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2e, 0x43, 0x6f, 0x6e, 0x66,
	0x69, 0x67, 0x52, 0x07, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x73, 0x22, 0xb0, 0x01, 0x0a, 0x13,
	0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x27, 0x0a, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69,
	0x67, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70,
	0x62, 0x2e, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x12, 0x3b, 0x0a, 0x0b, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x5f, 0x6d, 0x61, 0x73, 0x6b, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46, 0x69, 0x65, 0x6c, 0x64, 0x4d, 0x61, 0x73,
	0x6b, 0x52, 0x0a, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x4d, 0x61, 0x73, 0x6b, 0x12, 0x1f, 0x0a,
	0x0b, 0x72, 0x65, 0x73, 0x65, 0x74, 0x5f, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x18, 0x04, 0x20, 0x01,
	0x28, 0x08, 0x52, 0x0a, 0x72, 0x65, 0x73, 0x65, 0x74, 0x51, 0x75, 0x6f, 0x74, 0x61, 0x32, 0x95,
	0x04, 0x0a, 0x05, 0x51, 0x75, 0x6f, 0x74, 0x61, 0x12, 0x6a, 0x0a, 0x0c, 0x43, 0x72, 0x65, 0x61,
	0x74, 0x65, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x1c, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61,
	0x70, 0x62, 0x2e, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0f, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62,
	0x2e, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x22, 0x2b, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x25, 0x22,
	0x20, 0x2f, 0x76, 0x31, 0x62, 0x65, 0x74, 0x61, 0x31, 0x2f, 0x7b, 0x6e, 0x61, 0x6d, 0x65, 0x3d,
	0x71, 0x75, 0x6f, 0x74, 0x61, 0x73, 0x2f, 0x2a, 0x2a, 0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x7d, 0x3a, 0x01, 0x2a, 0x12, 0x6e, 0x0a, 0x0c, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x43, 0x6f,
	0x6e, 0x66, 0x69, 0x67, 0x12, 0x1c, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2e, 0x44,
	0x65, 0x6c, 0x65, 0x74, 0x65, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x28, 0x82, 0xd3, 0xe4, 0x93,
	0x02, 0x22, 0x2a, 0x20, 0x2f, 0x76, 0x31, 0x62, 0x65, 0x74, 0x61, 0x31, 0x2f, 0x7b, 0x6e, 0x61,
	0x6d, 0x65, 0x3d, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x73, 0x2f, 0x2a, 0x2a, 0x2f, 0x63, 0x6f, 0x6e,
	0x66, 0x69, 0x67, 0x7d, 0x12, 0x61, 0x0a, 0x09, 0x47, 0x65, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69,
	0x67, 0x12, 0x19, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2e, 0x47, 0x65, 0x74, 0x43,
	0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0f, 0x2e, 0x71,
	0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2e, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x22, 0x28, 0x82,
	0xd3, 0xe4, 0x93, 0x02, 0x22, 0x12, 0x20, 0x2f, 0x76, 0x31, 0x62, 0x65, 0x74, 0x61, 0x31, 0x2f,
	0x7b, 0x6e, 0x61, 0x6d, 0x65, 0x3d, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x73, 0x2f, 0x2a, 0x2a, 0x2f,
	0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x7d, 0x12, 0x61, 0x0a, 0x0b, 0x4c, 0x69, 0x73, 0x74, 0x43,
	0x6f, 0x6e, 0x66, 0x69, 0x67, 0x73, 0x12, 0x1b, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62,
	0x2e, 0x4c, 0x69, 0x73, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x73, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x1c, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2e, 0x4c, 0x69,
	0x73, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x22, 0x17, 0x82, 0xd3, 0xe4, 0x93, 0x02, 0x11, 0x12, 0x0f, 0x2f, 0x76, 0x31, 0x62, 0x65,
	0x74, 0x61, 0x31, 0x2f, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x73, 0x12, 0x6a, 0x0a, 0x0c, 0x55, 0x70,
	0x64, 0x61, 0x74, 0x65, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x1c, 0x2e, 0x71, 0x75, 0x6f,
	0x74, 0x61, 0x70, 0x62, 0x2e, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x43, 0x6f, 0x6e, 0x66, 0x69,
	0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0f, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61,
	0x70, 0x62, 0x2e, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x22, 0x2b, 0x82, 0xd3, 0xe4, 0x93, 0x02,
	0x25, 0x32, 0x20, 0x2f, 0x76, 0x31, 0x62, 0x65, 0x74, 0x61, 0x31, 0x2f, 0x7b, 0x6e, 0x61, 0x6d,
	0x65, 0x3d, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x73, 0x2f, 0x2a, 0x2a, 0x2f, 0x63, 0x6f, 0x6e, 0x66,
	0x69, 0x67, 0x7d, 0x3a, 0x01, 0x2a, 0x42, 0x2f, 0x5a, 0x2d, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x74, 0x72, 0x69, 0x6c,
	0x6c, 0x69, 0x61, 0x6e, 0x2f, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x2f, 0x65, 0x74, 0x63, 0x64, 0x2f,
	0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_quotapb_proto_rawDescOnce sync.Once
	file_quotapb_proto_rawDescData = file_quotapb_proto_rawDesc
)

func file_quotapb_proto_rawDescGZIP() []byte {
	file_quotapb_proto_rawDescOnce.Do(func() {
		file_quotapb_proto_rawDescData = protoimpl.X.CompressGZIP(file_quotapb_proto_rawDescData)
	})
	return file_quotapb_proto_rawDescData
}

var file_quotapb_proto_enumTypes = make([]protoimpl.EnumInfo, 2)
var file_quotapb_proto_msgTypes = make([]protoimpl.MessageInfo, 9)
var file_quotapb_proto_goTypes = []interface{}{
	(Config_State)(0),                // 0: quotapb.Config.State
	(ListConfigsRequest_ListView)(0), // 1: quotapb.ListConfigsRequest.ListView
	(*Config)(nil),                   // 2: quotapb.Config
	(*SequencingBasedStrategy)(nil),  // 3: quotapb.SequencingBasedStrategy
	(*TimeBasedStrategy)(nil),        // 4: quotapb.TimeBasedStrategy
	(*CreateConfigRequest)(nil),      // 5: quotapb.CreateConfigRequest
	(*DeleteConfigRequest)(nil),      // 6: quotapb.DeleteConfigRequest
	(*GetConfigRequest)(nil),         // 7: quotapb.GetConfigRequest
	(*ListConfigsRequest)(nil),       // 8: quotapb.ListConfigsRequest
	(*ListConfigsResponse)(nil),      // 9: quotapb.ListConfigsResponse
	(*UpdateConfigRequest)(nil),      // 10: quotapb.UpdateConfigRequest
	(*field_mask.FieldMask)(nil),     // 11: google.protobuf.FieldMask
	(*empty.Empty)(nil),              // 12: google.protobuf.Empty
}
var file_quotapb_proto_depIdxs = []int32{
	0,  // 0: quotapb.Config.state:type_name -> quotapb.Config.State
	3,  // 1: quotapb.Config.sequencing_based:type_name -> quotapb.SequencingBasedStrategy
	4,  // 2: quotapb.Config.time_based:type_name -> quotapb.TimeBasedStrategy
	2,  // 3: quotapb.CreateConfigRequest.config:type_name -> quotapb.Config
	1,  // 4: quotapb.ListConfigsRequest.view:type_name -> quotapb.ListConfigsRequest.ListView
	2,  // 5: quotapb.ListConfigsResponse.configs:type_name -> quotapb.Config
	2,  // 6: quotapb.UpdateConfigRequest.config:type_name -> quotapb.Config
	11, // 7: quotapb.UpdateConfigRequest.update_mask:type_name -> google.protobuf.FieldMask
	5,  // 8: quotapb.Quota.CreateConfig:input_type -> quotapb.CreateConfigRequest
	6,  // 9: quotapb.Quota.DeleteConfig:input_type -> quotapb.DeleteConfigRequest
	7,  // 10: quotapb.Quota.GetConfig:input_type -> quotapb.GetConfigRequest
	8,  // 11: quotapb.Quota.ListConfigs:input_type -> quotapb.ListConfigsRequest
	10, // 12: quotapb.Quota.UpdateConfig:input_type -> quotapb.UpdateConfigRequest
	2,  // 13: quotapb.Quota.CreateConfig:output_type -> quotapb.Config
	12, // 14: quotapb.Quota.DeleteConfig:output_type -> google.protobuf.Empty
	2,  // 15: quotapb.Quota.GetConfig:output_type -> quotapb.Config
	9,  // 16: quotapb.Quota.ListConfigs:output_type -> quotapb.ListConfigsResponse
	2,  // 17: quotapb.Quota.UpdateConfig:output_type -> quotapb.Config
	13, // [13:18] is the sub-list for method output_type
	8,  // [8:13] is the sub-list for method input_type
	8,  // [8:8] is the sub-list for extension type_name
	8,  // [8:8] is the sub-list for extension extendee
	0,  // [0:8] is the sub-list for field type_name
}

func init() { file_quotapb_proto_init() }
func file_quotapb_proto_init() {
	if File_quotapb_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_quotapb_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Config); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_quotapb_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SequencingBasedStrategy); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_quotapb_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*TimeBasedStrategy); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_quotapb_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CreateConfigRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_quotapb_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*DeleteConfigRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_quotapb_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetConfigRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_quotapb_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ListConfigsRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_quotapb_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ListConfigsResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_quotapb_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*UpdateConfigRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	file_quotapb_proto_msgTypes[0].OneofWrappers = []interface{}{
		(*Config_SequencingBased)(nil),
		(*Config_TimeBased)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_quotapb_proto_rawDesc,
			NumEnums:      2,
			NumMessages:   9,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_quotapb_proto_goTypes,
		DependencyIndexes: file_quotapb_proto_depIdxs,
		EnumInfos:         file_quotapb_proto_enumTypes,
		MessageInfos:      file_quotapb_proto_msgTypes,
	}.Build()
	File_quotapb_proto = out.File
	file_quotapb_proto_rawDesc = nil
	file_quotapb_proto_goTypes = nil
	file_quotapb_proto_depIdxs = nil
}
