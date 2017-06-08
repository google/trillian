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

package signer

import "encoding/json"

// STH is a signed tree head.
type STH struct {
	TreeSize  int   `json:"sz"`
	TimeStamp int64 `json:"tm"`
	// Store the offset that the STH is supposed to appear at.  Entries where this
	// does not match the actual offset should be ignored.
	Offset int `json:"off"`
}

func (s *STH) String() string {
	v, err := json.Marshal(*s)
	if err != nil {
		panic(err)
	}
	return string(v)
}

func sthFromString(s string) *STH {
	if s == "" {
		return nil
	}
	var result STH
	json.Unmarshal([]byte(s), &result)
	return &result
}

// STHInfo holds information about an STH stored in the STH topic.
type STHInfo struct {
	treeRevision int
	sthOffset    int
	sth          STH
}
