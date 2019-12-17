// Copyright 2019 Google Inc. All Rights Reserved.
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

// +build tools

// Package tools tracks dependencies on binaries not otherwise referenced in the trillian codebase.
// https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
package tools

import (
	_ "github.com/golang/mock/gomock"
	_ "github.com/golang/mock/mockgen"
	_ "github.com/golang/protobuf/proto"
	_ "github.com/golang/protobuf/protoc-gen-go"
	_ "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway"
	_ "github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc"
	_ "github.com/uber/prototool/cmd/prototool"
	_ "go.etcd.io/etcd"
	_ "go.etcd.io/etcd/etcdctl"
	_ "golang.org/x/tools/cmd/stringer"
)
