// Copyright 2024 Trillian Authors. All Rights Reserved.
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

package postgresql

import (
	"flag"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly/flagsaver"
)

func TestPostgreSQLStorageProviderErrorPersistence(t *testing.T) {
	defer flagsaver.Save().MustRestore()
	if err := flag.Set("postgresql_uri", "&bogus*:::?"); err != nil {
		t.Errorf("Failed to set flag: %v", err)
	}

	// First call: This should fail due to the Database URL being garbage.
	_, err1 := storage.NewProvider("postgresql", nil)
	if err1 == nil {
		t.Fatalf("Expected 'storage.NewProvider' to fail")
	}

	// Second call: This should fail with the same error.
	_, err2 := storage.NewProvider("postgresql", nil)
	if err2 == nil {
		t.Fatalf("Expected second call to 'storage.NewProvider' to fail")
	}

	if err2 != err1 {
		t.Fatalf("Expected second call to 'storage.NewProvider' to fail with %q, instead got: %q", err1, err2)
	}
}

func TestBuildPostgresTLSURINoTLS(t *testing.T) {
	defer flagsaver.Save().MustRestore()
	originalURI := "postgresql:///defaultdb?host=localhost&user=test"
	if err := flag.Set("postgresql_tls_ca", ""); err != nil {
		t.Fatalf("Failed to set flag: %v", err)
	}

	modified, err := BuildPostgresTLSURI(originalURI)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if modified != originalURI {
		t.Errorf("Expected unmodified URI %q, got %q", originalURI, modified)
	}
}

func TestBuildPostgresTLSURIMissingCAFile(t *testing.T) {
	defer flagsaver.Save().MustRestore()

	if err := flag.Set("postgresql_tls_ca", "/path/does/not/exist.crt"); err != nil {
		t.Fatalf("Failed to set flag: %v", err)
	}

	_, err := BuildPostgresTLSURI("postgresql:///defaultdb")
	if err == nil {
		t.Fatalf("Expected error due to missing CA file, got nil")
	}
	if !strings.Contains(err.Error(), "CA file error") {
		t.Fatalf("Expected CA file error, got: %v", err)
	}
}

func TestBuildPostgresTLSURIVerifyCA(t *testing.T) {
	defer flagsaver.Save().MustRestore()

	tmp := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(tmp, []byte("test_ca_cert_content"), 0400); err != nil {
		t.Fatalf("Failed to write CA file: %v", err)
	}

	if err := flag.Set("postgresql_tls_ca", tmp); err != nil {
		t.Fatalf("Failed to set CA flag: %v", err)
	}
	if err := flag.Set("postgresql_verify_full", "false"); err != nil {
		t.Fatalf("Failed to set verify flag: %v", err)
	}

	original := "postgresql:///defaultdb?host=localhost&user=test"
	modified, err := BuildPostgresTLSURI(original)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	u, err := url.Parse(modified)
	if err != nil {
		t.Fatalf("Failed to parse modified URI: %v", err)
	}
	q := u.Query()

	if q.Get("sslrootcert") != tmp {
		t.Errorf("Expected sslrootcert=%q, got %q", tmp, q.Get("sslrootcert"))
	}
	if q.Get("sslmode") != "verify-ca" {
		t.Errorf("Expected sslmode=verify-ca, got %q", q.Get("sslmode"))
	}
}

func TestBuildPostgresTLSURIVerifyFull(t *testing.T) {
	defer flagsaver.Save().MustRestore()

	tmp := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(tmp, []byte("test_ca_cert_content"), 0400); err != nil {
		t.Fatalf("Failed to write CA file: %v", err)
	}

	if err := flag.Set("postgresql_tls_ca", tmp); err != nil {
		t.Fatalf("Failed to set CA flag: %v", err)
	}
	if err := flag.Set("postgresql_verify_full", "true"); err != nil {
		t.Fatalf("Failed to set verify_full flag: %v", err)
	}

	original := "postgresql:///defaultdb?host=localhost&user=test"
	modified, err := BuildPostgresTLSURI(original)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	u, err := url.Parse(modified)
	if err != nil {
		t.Fatalf("Failed to parse modified URI: %v", err)
	}
	q := u.Query()

	if q.Get("sslrootcert") != tmp {
		t.Errorf("Expected sslrootcert=%q, got %q", tmp, q.Get("sslrootcert"))
	}
	if q.Get("sslmode") != "verify-full" {
		t.Errorf("Expected sslmode=verify-full, got %q", q.Get("sslmode"))
	}
}
