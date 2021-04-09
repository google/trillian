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

package keys

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// ProtoBuilder creates a protobuf message that describes a private key.
// It may use flags to obtain the required information, e.g. file paths.
type ProtoBuilder func() (proto.Message, error)

// protoBuilders is a map of key types to funcs that can create protobuf
// messages describing them. The "key type" is a human-friendly identifier;
// it does not have to be the name of the protobuf message.
var protoBuilders = make(map[string]ProtoBuilder)

// RegisterType registers a func that can create protobuf messages which describe
// a private key.
func RegisterType(protoType string, builder ProtoBuilder) {
	protoBuilders[protoType] = builder
}

// New returns a protobuf message of the specified type that describes a private
// key. A ProtoBuilder must have been registered for this type using
// RegisterType() first.
func New(protoType string) (*anypb.Any, error) {
	buildProto, ok := protoBuilders[protoType]
	if !ok {
		return nil, fmt.Errorf("key protobuf type must be one of: %s", strings.Join(RegisteredTypes(), ", "))
	}

	pb, err := buildProto()
	if err != nil {
		return nil, err
	}

	return anypb.New(pb)
}

// RegisteredTypes returns a list of protobuf message types that have been
// registered using RegisterType().
func RegisteredTypes() []string {
	protoTypes := make([]string, 0, len(protoBuilders))
	for protoType := range protoBuilders {
		protoTypes = append(protoTypes, protoType)
	}
	return protoTypes
}
