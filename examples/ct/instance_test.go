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

package ct

import (
	"strings"
	"testing"
	"time"
)

func TestSetUpInstance(t *testing.T) {
	var tests = []struct {
		desc   string
		cfg    LogConfig
		errStr string
	}{
		{
			desc: "valid",
			cfg: LogConfig{
				LogID:           1,
				Prefix:          "log",
				RootsPEMFile:    []string{"../../testdata/fake-ca.cert"},
				PrivKeyPEMFile:  "../../testdata/ct-http-server.privkey.pem",
				PrivKeyPassword: "dirk",
			},
		},
		{
			desc: "no-roots",
			cfg: LogConfig{
				LogID:           1,
				Prefix:          "log",
				PrivKeyPEMFile:  "../../testdata/ct-http-server.privkey.pem",
				PrivKeyPassword: "dirk",
			},
			errStr: "specify RootsPEMFile",
		},
		{
			desc: "no-priv-key",
			cfg: LogConfig{
				LogID:        1,
				Prefix:       "log",
				RootsPEMFile: []string{"../../testdata/fake-ca.cert"},
			},
			errStr: "specify PrivKeyPEMFile",
		},
		{
			desc: "missing-root-cert",
			cfg: LogConfig{
				LogID:           1,
				Prefix:          "log",
				RootsPEMFile:    []string{"../../testdata/bogus.cert"},
				PrivKeyPEMFile:  "../../testdata/ct-http-server.privkey.pem",
				PrivKeyPassword: "dirk",
			},
			errStr: "failed to read trusted roots",
		},
		{
			desc: "missing-privkey",
			cfg: LogConfig{
				LogID:           1,
				Prefix:          "log",
				RootsPEMFile:    []string{"../../testdata/fake-ca.cert"},
				PrivKeyPEMFile:  "../../testdata/bogus.privkey.pem",
				PrivKeyPassword: "dirk",
			},
			errStr: "failed to load private key",
		},
		{
			desc: "privkey-wrong-password",
			cfg: LogConfig{
				LogID:           1,
				Prefix:          "log",
				RootsPEMFile:    []string{"../../testdata/fake-ca.cert"},
				PrivKeyPEMFile:  "../../testdata/ct-http-server.privkey.pem",
				PrivKeyPassword: "dirkly",
			},
			errStr: "failed to load private key",
		},
		{
			desc: "valid-ekus-1",
			cfg: LogConfig{
				LogID:           1,
				Prefix:          "log",
				RootsPEMFile:    []string{"../../testdata/fake-ca.cert"},
				PrivKeyPEMFile:  "../../testdata/ct-http-server.privkey.pem",
				PrivKeyPassword: "dirk",
				ExtKeyUsages:    []string{"Any"},
			},
		},
		{
			desc: "valid-ekus-2",
			cfg: LogConfig{
				LogID:           1,
				Prefix:          "log",
				RootsPEMFile:    []string{"../../testdata/fake-ca.cert"},
				PrivKeyPEMFile:  "../../testdata/ct-http-server.privkey.pem",
				PrivKeyPassword: "dirk",
				ExtKeyUsages:    []string{"Any", "ServerAuth", "TimeStamping"},
			},
		},
		{
			desc: "invalid-ekus-1",
			cfg: LogConfig{
				LogID:           1,
				Prefix:          "log",
				RootsPEMFile:    []string{"../../testdata/fake-ca.cert"},
				PrivKeyPEMFile:  "../../testdata/ct-http-server.privkey.pem",
				PrivKeyPassword: "dirk",
				ExtKeyUsages:    []string{"Any", "ServerAuth", "TimeStomping"},
			},
			errStr: "unknown extended key usage",
		},
		{
			desc: "invalid-ekus-2",
			cfg: LogConfig{
				LogID:           1,
				Prefix:          "log",
				RootsPEMFile:    []string{"../../testdata/fake-ca.cert"},
				PrivKeyPEMFile:  "../../testdata/ct-http-server.privkey.pem",
				PrivKeyPassword: "dirk",
				ExtKeyUsages:    []string{"Any "},
			},
			errStr: "unknown extended key usage",
		},
	}

	for _, test := range tests {
		_, err := test.cfg.SetUpInstance(nil, time.Second)
		if err != nil {
			if test.errStr == "" {
				t.Errorf("(%v).SetUpInstance()=_,%v; want _,nil", test.desc, err)
			} else if !strings.Contains(err.Error(), test.errStr) {
				t.Errorf("(%v).SetUpInstance()=_,%v; want err containing %q", test.desc, err, test.errStr)
			}
			continue
		}
		if test.errStr != "" {
			t.Errorf("(%v).SetUpInstance()=_,mo;; want err containing %q", test.desc, test.errStr)
		}
	}

}
