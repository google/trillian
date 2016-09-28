package main

import (
	"reflect"
	"testing"

	"github.com/google/certificate-transparency/go/x509"
	"github.com/google/certificate-transparency/go/x509/pkix"
	"github.com/google/trillian/examples/ct/ct_mapper"
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

	expected := map[string]ct_mapper.EntryList{
		"commonName":  ct_mapper.EntryList{Domain: "commonName", CertIndex: []int64{0, 10, 11, 12, 13}},
		"anotherName": ct_mapper.EntryList{Domain: "anotherName", CertIndex: []int64{20}, PrecertIndex: []int64{21}},
		"alt1":        ct_mapper.EntryList{Domain: "alt1", CertIndex: []int64{20}, PrecertIndex: []int64{21}},
		"alt2":        ct_mapper.EntryList{Domain: "alt2", CertIndex: []int64{20}, PrecertIndex: []int64{21}},
	}

	m := make(map[string]ct_mapper.EntryList)

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

	if !reflect.DeepEqual(m, expected) {
		t.Fatalf("Built incorrect map:\n%#v\nexpected:\n%#v", m, expected)
	}
}
