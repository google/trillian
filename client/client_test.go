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

package client

import (
	"testing"

	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/testonly/integration"
)

func TestAddLeaf(t *testing.T) {
	logID := int64(1234)
	env, err := integration.NewLogEnv("client")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	if err := mysql.CreateTree(logID, env.DB); err != nil {
		t.Errorf("Failed to create log: %v", err)
	}

	client := New(logID, env.ClientConn)

	if err := client.AddLeaf([]byte("foo")); err != nil {
		t.Errorf("Failed to add Leaf: %v", err)
	}
}

func TestAddSameLeaf(t *testing.T) {
	logID := int64(1234)
	t.Skip("Submitting two leaves currently breaks")
	env, err := integration.NewLogEnv("client")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	client := New(logID, env.ClientConn)

	if err := mysql.CreateTree(logID, env.DB); err != nil {
		t.Errorf("Failed to create log: %v", err)
	}
	if err := client.AddLeaf([]byte("foo")); err != nil {
		t.Error(err)
	}
	if err := client.AddLeaf([]byte("foo")); err != nil {
		t.Error(err)
	}
}
