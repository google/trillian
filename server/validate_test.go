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

package server

import (
	"testing"

	"github.com/google/trillian"
)

func TestGetConsistencyProofInvalidRequest(t *testing.T) {
	for _, test := range []struct {
		cReq *trillian.GetConsistencyProofRequest
	}{
		{
			&trillian.GetConsistencyProofRequest{
				LogId:          logID1,
				FirstTreeSize:  -10,
				SecondTreeSize: 25,
			},
		},
		{
			&trillian.GetConsistencyProofRequest{
				LogId:          logID1,
				FirstTreeSize:  10,
				SecondTreeSize: -25,
			},
		},
		{
			&trillian.GetConsistencyProofRequest{
				LogId:          logID1,
				FirstTreeSize:  330,
				SecondTreeSize: 329,
			},
		},
	} {
		if err := validateGetConsistencyProofRequest(test.cReq); err == nil {
			t.Errorf("validateGetConsistencyProofRequest(%v): %v, want nil", test.cReq, err)
		}
	}
}

func TestGetEntryAndProofInvalidRequest(t *testing.T) {
	for _, test := range []struct {
		pReq *trillian.GetEntryAndProofRequest
	}{
		{pReq: &trillian.GetEntryAndProofRequest{
			LogId:     logID1,
			TreeSize:  -20,
			LeafIndex: 20,
		}},
		{pReq: &trillian.GetEntryAndProofRequest{
			LogId:     logID1,
			TreeSize:  25,
			LeafIndex: -5,
		}},
		{pReq: &trillian.GetEntryAndProofRequest{
			LogId:     logID1,
			TreeSize:  25,
			LeafIndex: 30,
		}},
	} {
		if err := validateGetEntryAndProofRequest(test.pReq); err == nil {
			t.Errorf("validateGetEntryAndProofRequest(%v): %v, want nil", test.pReq, err)
		}
	}
}

func TestGetInclusionProofRequest(t *testing.T) {
	for _, test := range []struct {
		iReq *trillian.GetInclusionProofRequest
	}{
		{
			iReq: &trillian.GetInclusionProofRequest{
				LogId:     logID1,
				TreeSize:  -50,
				LeafIndex: 10,
			},
		},
		{
			iReq: &trillian.GetInclusionProofRequest{
				LogId:     logID1,
				TreeSize:  50,
				LeafIndex: -10,
			},
		},
		{
			iReq: &trillian.GetInclusionProofRequest{
				LogId:     logID1,
				TreeSize:  50,
				LeafIndex: 60,
			},
		},
	} {
		if err := validateGetInclusionProofRequest(test.iReq); err == nil {
			t.Errorf("verifyGetInclusionProofRequest(%v): %v, want nil",
				test.iReq, err)
		}
	}
}

func TestGetProofByHashInvalidRequests(t *testing.T) {
	for _, test := range []struct {
		hReq *trillian.GetInclusionProofByHashRequest
	}{
		{
			hReq: &trillian.GetInclusionProofByHashRequest{
				LogId:    logID1,
				TreeSize: -50,
				LeafHash: []byte("data"),
			},
		},
		{
			hReq: &trillian.GetInclusionProofByHashRequest{
				LogId:    logID1,
				TreeSize: 50,
				LeafHash: []byte{},
			},
		},
	} {
		if err := validateGetInclusionProofByHashRequest(test.hReq); err == nil {
			t.Errorf("verifyGetInclusionProofByHash(%v): %v, want nil",
				test.hReq, err)
		}
	}
}
