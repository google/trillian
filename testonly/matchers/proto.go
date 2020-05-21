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

package matchers

import (
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
)

type protoEqual struct {
	msg proto.Message
}

// ProtoEqual returns a matcher that compares protobuf messages.
func ProtoEqual(m proto.Message) gomock.Matcher {
	return protoEqual{msg: m}
}

// Matches implements the gomock.Matcher API.
func (pe protoEqual) Matches(msg interface{}) bool {
	m, ok := msg.(proto.Message)
	if !ok {
		return false
	}
	return proto.Equal(m, pe.msg)
}

func (pe protoEqual) String() string {
	return fmt.Sprintf("is equal to %s", pe.msg)
}
