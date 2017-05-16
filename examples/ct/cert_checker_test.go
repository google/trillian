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
	"encoding/pem"
	"testing"

	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/certificate-transparency-go/x509/pkix"
	"github.com/google/trillian/examples/ct/testonly"
)

func wipeExtensions(cert *x509.Certificate) *x509.Certificate {
	cert.Extensions = cert.Extensions[:0]
	return cert
}

func makePoisonNonCritical(cert *x509.Certificate) *x509.Certificate {
	// Invalid as a pre-cert because poison extension needs to be marked as critical.
	cert.Extensions = []pkix.Extension{{Id: ctPoisonExtensionOID, Critical: false, Value: asn1NullBytes}}
	return cert
}

func makePoisonNonNull(cert *x509.Certificate) *x509.Certificate {
	// Invalid as a pre-cert because poison extension is not ASN.1 NULL value.
	cert.Extensions = []pkix.Extension{{Id: ctPoisonExtensionOID, Critical: false, Value: []byte{0x42, 0x42, 0x42}}}
	return cert
}

func TestIsPrecertificate(t *testing.T) {
	var tests = []struct {
		desc        string
		cert        *x509.Certificate
		wantPrecert bool
		wantErr     bool
	}{
		{
			desc:        "valid-precert",
			cert:        pemToCert(t, testonly.PrecertPEMValid),
			wantPrecert: true,
		},
		{
			desc:        "valid-cert",
			cert:        pemToCert(t, testonly.CACertPEM),
			wantPrecert: false,
		},
		{
			desc:        "remove-exts-from-precert",
			cert:        wipeExtensions(pemToCert(t, testonly.PrecertPEMValid)),
			wantPrecert: false,
		},
		{
			desc:        "poison-non-critical",
			cert:        makePoisonNonCritical(pemToCert(t, testonly.PrecertPEMValid)),
			wantPrecert: false,
			wantErr:     true,
		},
		{
			desc:        "poison-non-null",
			cert:        makePoisonNonNull(pemToCert(t, testonly.PrecertPEMValid)),
			wantPrecert: false,
			wantErr:     true,
		},
	}

	for _, test := range tests {
		gotPrecert, err := IsPrecertificate(test.cert)
		if err != nil {
			if !test.wantErr {
				t.Errorf("IsPrecertificate(%v)=%v,%v; want %v,nil", test.desc, gotPrecert, err, test.wantPrecert)
			}
			continue
		}
		if test.wantErr {
			t.Errorf("IsPrecertificate(%v)=%v,%v; want _,%v", test.desc, gotPrecert, err, test.wantErr)
		}
		if gotPrecert != test.wantPrecert {
			t.Errorf("IsPrecertificate(%v)=%v,%v; want %v,nil", test.desc, gotPrecert, err, test.wantPrecert)
		}
	}
}

func TestValidateChain(t *testing.T) {
	fakeCARoots := NewPEMCertPool()
	if !fakeCARoots.AppendCertsFromPEM([]byte(testonly.FakeCACertPEM)) {
		t.Fatal("failed to load fake root")
	}
	validateOpts := CertValidationOpts{
		trustedRoots:  fakeCARoots,
		rejectExpired: false,
		extKeyUsages:  []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	var tests = []struct {
		desc        string
		chain       [][]byte
		wantErr     bool
		wantPathLen int
	}{
		{
			desc:    "missing-intermediate-cert",
			chain:   pemsToDERChain(t, []string{testonly.LeafSignedByFakeIntermediateCertPEM}),
			wantErr: true,
		},
		{
			desc:    "wrong-cert-order",
			chain:   pemsToDERChain(t, []string{testonly.FakeIntermediateCertPEM, testonly.LeafSignedByFakeIntermediateCertPEM}),
			wantErr: true,
		},
		{
			desc:    "unrelated-cert-in-chain",
			chain:   pemsToDERChain(t, []string{testonly.FakeIntermediateCertPEM, testonly.TestCertPEM}),
			wantErr: true,
		},
		{
			desc:    "unrelated-cert-after-chain",
			chain:   pemsToDERChain(t, []string{testonly.LeafSignedByFakeIntermediateCertPEM, testonly.FakeIntermediateCertPEM, testonly.TestCertPEM}),
			wantErr: true,
		},
		{
			desc:        "valid-chain",
			chain:       pemsToDERChain(t, []string{testonly.LeafSignedByFakeIntermediateCertPEM, testonly.FakeIntermediateCertPEM}),
			wantPathLen: 3,
		},
	}
	for _, test := range tests {
		gotPath, err := ValidateChain(test.chain, validateOpts)
		if err != nil {
			if !test.wantErr {
				t.Errorf("ValidateChain(%v)=%v,%v; want _,nil", test.desc, gotPath, err)
			}
			continue
		}
		if test.wantErr {
			t.Errorf("ValidateChain(%v)=%v,%v; want _,non-nil", test.desc, gotPath, err)
		}
		if len(gotPath) != test.wantPathLen {
			t.Errorf("|ValidateChain(%v)|=%d; want %d", test.desc, len(gotPath), test.wantPathLen)
		}
	}
}

// Builds a chain of DER-encoded certs.
// Note: ordering is important
func pemsToDERChain(t *testing.T, pemCerts []string) [][]byte {
	chain := make([][]byte, 0, len(pemCerts))
	for _, pemCert := range pemCerts {
		cert := pemToCert(t, pemCert)
		chain = append(chain, cert.Raw)
	}
	return chain
}

func pemToCert(t *testing.T, pemData string) *x509.Certificate {
	bytes, rest := pem.Decode([]byte(pemData))
	if len(rest) > 0 {
		t.Fatalf("Extra data after PEM: %v", rest)
		return nil
	}

	cert, err := x509.ParseCertificate(bytes.Bytes)
	if err != nil {
		_, ok := err.(x509.NonFatalErrors)
		if !ok {
			t.Fatal(err)
			return nil
		}
	}

	return cert
}
