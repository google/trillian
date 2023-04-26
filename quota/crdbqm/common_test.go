// Copyright 2022 Trillian Authors
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

package crdbqm

import (
	"os"
	"testing"

	"github.com/cockroachdb/cockroach-go/v2/testserver"
	"github.com/google/trillian/storage/testdb"
	"k8s.io/klog/v2"
)

func TestMain(m *testing.M) {
	ts, err := testserver.NewTestServer()
	if err != nil {
		klog.Errorf("Failed to start test server: %v", err)
		os.Exit(1)
	}
	defer ts.Stop()

	// reset the test server URL path. By default cockroach sets it
	// to point to a default database, we don't want that.
	dburl := ts.PGURL()
	dburl.Path = "/"

	// Set the environment variable for the test server
	if err := os.Setenv(testdb.CockroachDBURIEnv, dburl.String()); err != nil {
		klog.Errorf("Failed to set CockroachDBURIEnv: %v", err)
		os.Exit(1)
	}

	if !testdb.CockroachDBAvailable() {
		klog.Errorf("CockroachDB not available, skipping all CockroachDB storage tests")
		return
	}

	status := m.Run()

	os.Exit(status)
}
