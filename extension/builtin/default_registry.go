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

package builtin

import (
	"context"
	"crypto"
	"database/sql"
	"flag"
	"fmt"

	_ "github.com/go-sql-driver/mysql" // Load MySQL driver

	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"
)

var (
	// MySQLURIFlag is the mysql db connection string.
	MySQLURIFlag = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "uri to use with mysql storage")
)

// Default implementation of extension.Registry.
type defaultRegistry struct {
	db *sql.DB
}

func (r *defaultRegistry) GetAdminStorage() storage.AdminStorage {
	return mysql.NewAdminStorage(r.db)
}

func (r *defaultRegistry) GetLogStorage() (storage.LogStorage, error) {
	return mysql.NewLogStorage(r.db), nil
}

func (r *defaultRegistry) GetMapStorage() (storage.MapStorage, error) {
	return mysql.NewMapStorage(r.db), nil
}

func (r *defaultRegistry) GetKeyProvider() (keys.Provider, error) {
	return keyProvider{}, nil
}

// NewExtensionRegistry returns an extension.Registry implementation backed by a given
// MySQL database. It supports loading private keys from PEM files.
func NewExtensionRegistry(db *sql.DB) (extension.Registry, error) {
	return &defaultRegistry{db: db}, nil
}

// NewDefaultExtensionRegistry returns the default extension.Registry implementation, which is
// backed by a MySQL database and configured via flags.
// It supports loading private keys from PEM files.
func NewDefaultExtensionRegistry() (extension.Registry, error) {
	db, err := mysql.OpenDB(*MySQLURIFlag)
	if err != nil {
		return nil, err
	}
	return NewExtensionRegistry(db)
}

// keyProvider implements keys.Provider.
type keyProvider struct{}

// Signer returns a crypto.Signer for the given tree.
func (p keyProvider) Signer(ctx context.Context, tree *trillian.Tree) (crypto.Signer, error) {
	if tree.PrivateKey == nil {
		return nil, fmt.Errorf("tree %d has no PrivateKey", tree.GetTreeId())
	}

	var privateKey ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(tree.PrivateKey, &privateKey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal private key for tree %d: %v", tree.GetTreeId(), err)
	}

	switch privateKey := privateKey.Message.(type) {
	case *trillian.PEMKeyFile:
		return keys.NewFromPrivatePEMFile(privateKey.Path, privateKey.Password)
	}

	return nil, fmt.Errorf("unsupported PrivateKey type for tree %d: %T", tree.GetTreeId(), privateKey.Message)
}
