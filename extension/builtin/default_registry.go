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
	"database/sql"
	"flag"

	_ "github.com/go-sql-driver/mysql" // Load MySQL driver

	"github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"
)

var (
	// MySQLURIFlag is the mysql db connection string.
	MySQLURIFlag = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "uri to use with mysql storage")
	// TODO(Martin2112): Single private key doesn't really work for multi tenant and we can't use
	// an HSM interface in this way. Deferring these issues for later.
	privateKeyFile     = flag.String("private_key_file", "", "File containing a PEM encoded private key")
	privateKeyPassword = flag.String("private_key_password", "", "Password for server private key")
)

// Default implementation of extension.Registry.
type defaultRegistry struct {
	db     *sql.DB
	signer *crypto.Signer
}

func (r *defaultRegistry) GetLogStorage() (storage.LogStorage, error) {
	return mysql.NewLogStorage(r.db), nil
}

func (r *defaultRegistry) GetMapStorage() (storage.MapStorage, error) {
	return mysql.NewMapStorage(r.db), nil
}

func (r *defaultRegistry) GetSigner(treeID int64) (*crypto.Signer, error) {
	return r.signer, nil
}

// NewExtensionRegistry returns an extension.Registry implementation backed by a given
// MySQL database and signer.
func NewExtensionRegistry(db *sql.DB, signer *crypto.Signer) (extension.Registry, error) {
	return &defaultRegistry{db: db, signer: signer}, nil

}

// NewDefaultExtensionRegistry returns the default extension.Registry implementation, which is
// backed by a MySQL database and configured via flags.
func NewDefaultExtensionRegistry() (extension.Registry, error) {
	db, err := mysql.OpenDB(*MySQLURIFlag)
	if err != nil {
		return nil, err
	}
	key, err := keys.NewFromPrivatePEMFile(*privateKeyFile, *privateKeyPassword)
	if err != nil {
		return nil, err
	}
	return NewExtensionRegistry(db, crypto.NewSigner(key))
}
