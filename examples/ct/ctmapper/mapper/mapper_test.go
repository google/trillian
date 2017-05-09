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

package main

import (
	"testing"

	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/certificate-transparency-go/x509/pkix"
	"github.com/google/trillian/examples/ct/ctmapper/ctmapperpb"
	"github.com/kylelemons/godebug/pretty"
)

func TestUpdateDomainMap(t *testing.T) {
	vector := []struct {
		commonName   string
		subjectNames []string
		index        int64
		precert      bool
	}{
		{"commonName", nil, 0, false},
		{"commonName", nil, 10, false},
		{"", []string{"commonName"}, 11, false},
		{"commonName", []string{"commonName"}, 12, false},
		{"", []string{"commonName", "commonName"}, 13, false},

		{"anotherName", []string{"alt1", "alt2"}, 20, false},
		{"anotherName", []string{"alt1", "alt2"}, 21, true},
		{"", []string{"", ""}, 30, false},
	}

	expected := map[string]ctmapperpb.EntryList{
		"commonName":  {Domain: "commonName", CertIndex: []int64{0, 10, 11, 12, 13}},
		"anotherName": {Domain: "anotherName", CertIndex: []int64{20}, PrecertIndex: []int64{21}},
		"alt1":        {Domain: "alt1", CertIndex: []int64{20}, PrecertIndex: []int64{21}},
		"alt2":        {Domain: "alt2", CertIndex: []int64{20}, PrecertIndex: []int64{21}},
	}

	m := make(map[string]ctmapperpb.EntryList)

	for _, v := range vector {
		c := x509.Certificate{}
		if len(v.commonName) > 0 {
			c.Subject = pkix.Name{CommonName: v.commonName}
		}
		if len(v.subjectNames) > 0 {
			c.DNSNames = v.subjectNames
		}
		updateDomainMap(m, c, v.index, v.precert)
	}

	if diff := pretty.Compare(m, expected); diff != "" {
		t.Fatalf("Built incorrect map, diff:\n%v", diff)
	}
}
