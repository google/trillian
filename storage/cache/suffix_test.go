// Copyright 2017 Google LLC. All Rights Reserved.
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

package cache

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	_ "github.com/golang/glog" // Enable glog flags.
)

func h2b(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	return b
}

// h2b6 takes a hex string and emits a base64 string.
func h2b6(h string) string {
	return base64.StdEncoding.EncodeToString(h2b(h))
}

func TestParseSuffix(t *testing.T) {
	for _, tc := range []struct {
		str     string
		bits    byte
		path    []byte
		wantErr bool
	}{
		{str: h2b6(""), wantErr: true},
		// TODO(pavelkalinnikov): Parse "00" without a segfault in NewSuffix.
		{str: h2b6("0100"), bits: 1, path: h2b("00")},
		// TODO(pavelkalinnikov): The last byte must be masked.
		{str: h2b6("01FC"), bits: 1, path: h2b("FC")},
		{str: h2b6("010123"), wantErr: true},
		{str: h2b6("080123"), wantErr: true},
		{str: h2b6("0801"), bits: 8, path: h2b("01")},
		{str: h2b6("090123"), bits: 9, path: h2b("0123")},
		{str: h2b6("1000"), wantErr: true},
		{str: h2b6("100123"), bits: 16, path: h2b("0123")},
		{str: h2b6("2001234567"), bits: 32, path: h2b("01234567")},
		{str: "----", wantErr: true},
	} {
		t.Run("", func(t *testing.T) {
			sfx, err := ParseSuffix(tc.str)
			if got, want := err != nil, tc.wantErr; got != want {
				t.Fatalf("ParseSuffix: %v, wantErr: %v", err, want)
			} else if err != nil {
				return
			}
			if got, want := sfx.Bits(), tc.bits; got != want {
				t.Errorf("ParseSuffix: got %d bits, want %d", got, want)
			}
			if got, want := sfx.Path(), tc.path; !bytes.Equal(got, want) {
				t.Errorf("ParseSuffix: got path %x, want %x", got, want)
			}
		})
	}
}

// TestSuffixKey documents the behavior of makeSuffixKey
func TestSuffixKey(t *testing.T) {
	for _, tc := range []struct {
		depth   int
		index   int64
		want    []byte
		wantErr bool
	}{
		{depth: 0, index: 0x00, want: h2b("0000"), wantErr: false},
		{depth: 8, index: 0x00, want: h2b("0800"), wantErr: false},
		{depth: 15, index: 0xab, want: h2b("0fab"), wantErr: false},

		// Map cases which produce incorrect output from makeSuffixKey.
		{depth: 16, index: 0x00, want: h2b("1000"), wantErr: false},
		{depth: 8, index: 0xabcd, want: h2b("08cd"), wantErr: false},
		{
			depth:   2,
			index:   new(big.Int).SetBytes(h2b("4000000000000000000000000000000000000000000000000000000000000000")).Int64(),
			want:    h2b("0200"),
			wantErr: false,
		},
	} {
		suffixKey, err := makeSuffixKey(tc.depth, tc.index)
		if got, want := err != nil, tc.wantErr; got != want {
			t.Errorf("makeSuffixKey(%v, %v): %v, want err: %v",
				tc.depth, tc.index, err, want)
			continue
		}
		if err != nil {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(suffixKey)
		if err != nil {
			t.Errorf("DecodeString(%v): %v", suffixKey, err)
			continue
		}
		if got, want := b, tc.want; !bytes.Equal(got, want) {
			t.Errorf("makeSuffixKey(%v, %x): %x, want %x",
				tc.depth, tc.index, got, want)
		}
	}
}

// makeSuffixKey creates a suffix key for indexing into the subtree's Leaves and InternalNodes maps.
// This function documents existing log storage behavior. Any new code that emits Suffix objects must
// produce the exact same outputs as this function would for Logs.
func makeSuffixKey(depth int, index int64) (string, error) {
	if depth < 0 {
		return "", fmt.Errorf("invalid negative depth of %d", depth)
	}
	if index < 0 {
		return "", fmt.Errorf("invalid negative index %d", index)
	}
	sfx := NewSuffix(byte(depth), []byte{byte(index)})
	return sfx.String(), nil
}

func TestSuffixSerialize(t *testing.T) {
	for _, tc := range []struct {
		s    *Suffix
		want string
	}{
		// Pre-existing format. This test vector must NOT change or existing data will be inaccessible.
		{s: NewSuffix(1, []byte{0xae}), want: "Aa4="},
		{s: NewSuffix(5, []byte{0xae}), want: "Ba4="},
		{s: NewSuffix(8, []byte{0xae}), want: "CK4="},
		{s: NewSuffix(15, []byte{0xae, 0x27}), want: "D64n"},
		{s: NewSuffix(16, []byte{0xae, 0x27}), want: "EK4n"},
		{s: NewSuffix(23, []byte{0xae, 0x27, 0x49}), want: "F64nSQ=="},
	} {
		if got, want := tc.s.String(), tc.want; got != want {
			t.Errorf("%v.serialize(): %v, want %v", tc.s, got, want)
		}
	}
}

func TestSuffixPathImmutable(t *testing.T) {
	s1 := NewSuffix(8, []byte{0x97})
	s2 := NewSuffix(8, []byte{0x97})

	p1 := s1.Path()
	p2 := s2.Path()

	// Modifying the paths should leave the underlying objects still equal.
	p1[0] = 0xff
	if !bytes.Equal(s1.Path(), s2.Path()) {
		t.Errorf("suffix path is not immutable")
	}
	p2[0] = 0xff
	if !bytes.Equal(s1.Path(), s2.Path()) {
		t.Errorf("suffix path is not immutable")
	}
}

func Test8BitSuffixCache(t *testing.T) {
	for _, tc := range []struct {
		b         byte
		path      []byte
		wantCache bool
	}{
		// below 8 bits should be cached.
		{b: 1, path: []byte{0x80}, wantCache: true},
		{b: 1, path: []byte{0x40}, wantCache: false}, // bit set is outside the length.
		{b: 2, path: []byte{0x40}, wantCache: true},
		{b: 3, path: []byte{0x40}, wantCache: true},
		{b: 4, path: []byte{0x40}, wantCache: true},
		{b: 5, path: []byte{0x40}, wantCache: true},
		{b: 6, path: []byte{0x40}, wantCache: true},
		{b: 7, path: []byte{0x40}, wantCache: true},
		// 8 bits suffix should be cached.
		{b: 8, path: []byte{0x76}, wantCache: true},
		// above 8 bits should not be cached.
		{b: 9, path: []byte{0x40, 0x80}, wantCache: false},
		{b: 12, path: []byte{0x40, 0x80}, wantCache: false},
		{b: 15, path: []byte{0x40, 0xf0}, wantCache: false},
		{b: 24, path: []byte{0x40, 0xf0, 0xaa}, wantCache: false},
		{b: 32, path: []byte{0x40, 0xf0, 0xaa, 0xed}, wantCache: false},
	} {
		s1 := NewSuffix(tc.b, tc.path)
		s2 := NewSuffix(tc.b, tc.path)

		if s1 == s2 != tc.wantCache {
			t.Errorf("NewSuffix(): %v: cache / non cache mismatch: %v", tc, s1 == s2)
		}

		// Test the other direction as well by parsing it and we should get the
		// same instance again.
		s3, err := ParseSuffix(s1.String())
		if err != nil {
			t.Fatalf("failed to parse our own suffix: %v", err)
		}
		if s1 == s3 != tc.wantCache {
			t.Errorf("ParseSuffix(): %v: cache / non cache mismatch: %v", tc, s1 == s3)
		}
	}
}

// TestCacheIsolation ensures that users can't corrupt the cache by modifying
// values.
func TestCacheIsolation(t *testing.T) {
	s1 := NewSuffix(8, []byte{0x80})
	s1.Path()[0] ^= 0xff
	s2 := NewSuffix(8, []byte{0x80})

	if s1 != s2 {
		t.Fatalf("did not get same instance back from NewSuffix(8, ...)")
	}
	if s2.Path()[0] != 0x80 {
		t.Fatalf("cache instances are not immutable")
	}
}
