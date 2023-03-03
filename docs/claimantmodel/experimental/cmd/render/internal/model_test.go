// Copyright 2023 Trillian Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package claimant_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	claimant "github.com/google/trillian/docs/claimantmodel/experimental/cmd/render/internal"
	"gopkg.in/yaml.v2"
)

func TestModelAndQuestionnaire(t *testing.T) {
	testCases := []struct {
		system string
		terms  []string
	}{
		{
			system: "ct",
			terms:  []string{"$pubKey", "$domain"},
		},
		{
			system: "armorydrive",
			terms:  []string{"$artifactHash", "$platform", "$revision", "$git@tag", "$tamago@tag", "$usbarmory@tag"},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.system, func(t *testing.T) {
			saved, err := os.ReadFile(fmt.Sprintf("models/%s/model.yaml", tC.system))
			if err != nil {
				t.Fatalf("failed to read model: %v", err)
			}
			model := claimant.Model{}
			if err := yaml.Unmarshal(saved, &model); err != nil {
				t.Fatalf("failed to unmarshal model: %v", err)
			}
			if got, want := model.ClaimTerms(), tC.terms; !cmp.Equal(got, want) {
				t.Errorf("got != want; %v != %v", got, want)
			}
			saved, err = os.ReadFile(fmt.Sprintf("models/%s/model.md", tC.system))
			if err != nil {
				t.Fatalf("failed to read file: %v", err)
			}
			if diff := cmp.Diff(model.Markdown(), string(saved)); len(diff) != 0 {
				t.Errorf("unexpected diff: %v", diff)
			}
			saved, err = os.ReadFile(fmt.Sprintf("models/%s/questions.md", tC.system))
			if err != nil {
				t.Fatalf("failed to read file: %v", err)
			}
			// We allow the questions to be completed, but for the sake of diffing
			// we uncheck all of the boxes.
			unchecked := strings.ReplaceAll(string(saved), "[x]", "[ ]")
			if diff := cmp.Diff(model.Questionnaire(), unchecked); len(diff) != 0 {
				t.Errorf("unexpected diff: %v", diff)
			}
		})
	}
}
