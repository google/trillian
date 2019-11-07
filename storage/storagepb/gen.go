// Copyright 2016 Google Inc. All Rights Reserved.
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

// Package storagepb contains protobuf definitions used by various storage
// implementations.
//
// TODO(pavelkalinnikov, v2): SubtreeProto is used as:
//  a) database storage unit in multiple storage implementations;
//  b) data exchange format between storage and application layers;
//  c) nodes index data structure.
// We should change it so that:
//  a) individual storage implementations define their own formats;
//  b) data structures are defined in the application layer.
package storagepb

//go:generate protoc -I=. -I=$GOPATH/src/ --go_out=plugins=grpc:. storage.proto
