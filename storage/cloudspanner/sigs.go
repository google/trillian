// Copyright 2018 Google Inc. All Rights Reserved.
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

package cloudspanner

import (
	"fmt"

	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/storage/cloudspanner/spannerpb"
)

// apiToStorageSig converts a DigitallySigned proto from the API format to the Spanner storage format.
func apiToStorageSig(in *sigpb.DigitallySigned) (*spannerpb.DigitallySigned, error) {
	var hashAlg spannerpb.HashAlgorithm
	var sigAlg spannerpb.SignatureAlgorithm

	switch in.GetHashAlgorithm() {
	case sigpb.DigitallySigned_NONE:
		hashAlg = spannerpb.HashAlgorithm_NONE
	case sigpb.DigitallySigned_SHA256:
		hashAlg = spannerpb.HashAlgorithm_SHA256
	default:
		return nil, fmt.Errorf("apiToStorageSig: unknown HashAlgorithm %v", in.GetHashAlgorithm())
	}
	switch in.GetSignatureAlgorithm() {
	case sigpb.DigitallySigned_ANONYMOUS:
		sigAlg = spannerpb.SignatureAlgorithm_ANONYMOUS
	case sigpb.DigitallySigned_RSA:
		sigAlg = spannerpb.SignatureAlgorithm_RSA
	case sigpb.DigitallySigned_ECDSA:
		sigAlg = spannerpb.SignatureAlgorithm_ECDSA
	default:
		return nil, fmt.Errorf("apiToStorageSig: unknown SignatureAlgorithm %v", in.GetSignatureAlgorithm())
	}

	return &spannerpb.DigitallySigned{
		HashAlgorithm:      hashAlg,
		SignatureAlgorithm: sigAlg,
		Signature:          in.GetSignature(),
	}, nil
}

// storageToAPISig converts a DigitallySigned proto from the Spanner storage format to the API format.
func storageToAPISig(in *spannerpb.DigitallySigned) (*sigpb.DigitallySigned, error) {
	var hashAlg sigpb.DigitallySigned_HashAlgorithm
	var sigAlg sigpb.DigitallySigned_SignatureAlgorithm

	switch in.GetHashAlgorithm() {
	case spannerpb.HashAlgorithm_NONE:
		hashAlg = sigpb.DigitallySigned_NONE
	case spannerpb.HashAlgorithm_SHA256:
		hashAlg = sigpb.DigitallySigned_SHA256
	default:
		return nil, fmt.Errorf("storageToAPISig: unknown HashAlgorithm %v", in.GetHashAlgorithm())
	}
	switch in.GetSignatureAlgorithm() {
	case spannerpb.SignatureAlgorithm_ANONYMOUS:
		sigAlg = sigpb.DigitallySigned_ANONYMOUS
	case spannerpb.SignatureAlgorithm_RSA:
		sigAlg = sigpb.DigitallySigned_RSA
	case spannerpb.SignatureAlgorithm_ECDSA:
		sigAlg = sigpb.DigitallySigned_ECDSA
	default:
		return nil, fmt.Errorf("storageToAPISig: unknown SignatureAlgorithm %v", in.GetSignatureAlgorithm())
	}

	return &sigpb.DigitallySigned{
		HashAlgorithm:      hashAlg,
		SignatureAlgorithm: sigAlg,
		Signature:          in.GetSignature(),
	}, nil
}
