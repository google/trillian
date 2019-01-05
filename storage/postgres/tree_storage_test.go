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

package postgres

import (
	"strings"
	"testing"
)

type expandTestcase struct {
	input    *statementSkeleton
	expected string
}

// This test exists to prevent golangci-lint from failing
// unused functions.
// TODO(vishal): remove this once the rest of the storage code is complete.
func TestInitializes(t *testing.T) {
	_ = &statementSkeleton{}
	arbitraryStorage := newTreeStorage(nil)
	_ = arbitraryStorage.getSubtreeStmt
	_ = arbitraryStorage.beginTreeTx
	treeTx := &treeTX{}
	_ = treeTx.getSubtree
	_ = treeTx.getSubtrees
}

func TestExpandPlaceholderSQL(t *testing.T) {
	testCases := []*expandTestcase{
		{
			input: &statementSkeleton{
				sql:               selectSubtreeSQL,
				firstInsertion:    "%s",
				firstPlaceholders: 1,
				restInsertion:     "%s",
				restPlaceholders:  1,
				num:               2,
			},
			expected: strings.Replace(selectSubtreeSQL, placeholderSQL, "$1,$2", 1),
		},
		{
			input: &statementSkeleton{
				sql:               insertSubtreeMultiSQL,
				firstInsertion:    "VALUES(%s, %s, %s, %s)",
				firstPlaceholders: 4,
				restInsertion:     "(%s, %s, %s, %s)",
				restPlaceholders:  4,
				num:               2,
			},
			expected: strings.Replace(
				insertSubtreeMultiSQL,
				placeholderSQL,
				"VALUES($1, $2, $3, $4),($5, $6, $7, $8)",
				1),
		},
		{
			input: &statementSkeleton{
				sql:               selectSubtreeSQL,
				firstInsertion:    "%s",
				firstPlaceholders: 1,
				restInsertion:     "%s",
				restPlaceholders:  1,
				num:               5,
			},
			expected: strings.Replace(selectSubtreeSQL, placeholderSQL, "$1,$2,$3,$4,$5", 1),
		},
		{
			input: &statementSkeleton{
				sql:               insertSubtreeMultiSQL,
				firstInsertion:    "VALUES(%s, %s, %s, %s)",
				firstPlaceholders: 4,
				restInsertion:     "(%s, %s, %s, %s)",
				restPlaceholders:  4,
				num:               5,
			},
			expected: strings.Replace(
				insertSubtreeMultiSQL,
				placeholderSQL,
				"VALUES($1, $2, $3, $4),($5, $6, $7, $8),($9, $10, $11, $12),($13, $14, $15, $16),($17, $18, $19, $20)",
				1),
		},
	}

	for _, tc := range testCases {
		res, err := expandPlaceholderSQL(tc.input)
		if err != nil {
			t.Fatalf("Error while expanding placeholder sql: %v", err)
		}
		if tc.expected != res {
			t.Fatalf("Expected %v but got %v", tc.expected, res)
		}
	}
}
