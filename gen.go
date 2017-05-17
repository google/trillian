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

package trillian

//go:generate protoc -I=. -I=$GOPATH/src -I=$GOPATH/src/github.com/googleapis/googleapis --go_out=plugins=grpc:$GOPATH/src trillian_log_api.proto trillian_map_api.proto trillian_admin_api.proto trillian.proto
//go:generate protoc -I=. --go_out=:$GOPATH/src crypto/sigpb/sigpb.proto
//go:generate protoc -I=. --go_out=:$GOPATH/src crypto/keyspb/keyspb.proto
//go:generate protoc -I=. -I=$GOPATH/src -I=$GOPATH/src/github.com/google/trillian/vendor/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis -I=$GOPATH/src/github.com/googleapis/googleapis/ --grpc-gateway_out=logtostderr=true:$GOPATH/src trillian_log_api.proto trillian_map_api.proto trillian_admin_api.proto trillian.proto
