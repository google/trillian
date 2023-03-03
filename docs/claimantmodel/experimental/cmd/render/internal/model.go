// Copyright 2023 Trillian Authors. All Rights Reserved.
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

// Package claimant is a code model for the Claimant Model.
package claimant

import (
	"bytes"
	_ "embed"
	"fmt"
	"regexp"
	"sort"

	"text/template"
)

var (
	// TemplateModelMarkdown is the markdown template for a model.
	//go:embed tmpl_model.md
	TemplateModelMarkdown []byte

	// TemplateQuestions is the markdown template for a model questionnaire.
	//go:embed tmpl_questions.md
	TemplateQuestionsMarkdown []byte

	// TemplateSequence is the markdown template for the sequence diagram.
	//go:embed tmpl_sequence.md
	TemplateSequenceMarkdown []byte
)

// Claim represents a falsifiable statement along with an actor that can verify it.
type Claim struct {
	// Claim is a falsifiable statement, e.g. "The number 7 is prime"
	Claim string `yaml:"Claim"`
	// Verifier is an actor that can verify the claim, e.g. "Primal Calculator"
	Verifier string `yaml:"Verifier"`
}

// Model represents a Claimant Model, mapping the roles to actors.
// See https://github.com/google/trillian/blob/master/docs/claimantmodel/CoreModel.md for
// descriptions of these roles. Repeating the definitions here will only lead to stale
// documentation.
type Model struct {
	// System is a short upper case name that models the essence of this model, e.g. "FIRMWARE".
	System string `yaml:"System"`
	// Claimant is the actor playing the role of the Claimant.
	Claimant string `yaml:"Claimant"`
	// Statement is the concrete type that the Claimant issues, and is likely the thing that is logged.
	Statement string `yaml:"Statement"`

	// Believer is the actor playing the role of the Believer.
	Believer string `yaml:"Believer,omitempty"`
	// Believers are the actor playing the roles of the Believer.
	// This should only be provided if there are multiple Believers, and if
	// provided then Believer should be left empty.
	Believers []string `yaml:"Believers,omitempty"`

	// Claim is the claim made by the Claimant.
	Claim Claim `yaml:"Claim,omitempty"`
	// Claims are the claims made by the Claimant.
	// This should only be provided if there are multiple Claims, and if
	// provided then Claim should be left empty.
	Claims []Claim `yaml:"Claims,omitempty"`

	// Arbiter is the actor or process that fulfills the Arbiter role.
	Arbiter string `yaml:"Arbiter"`
}

// Markdown returns this Claimant Model in a definition table that renders
// clearly in markdown format.
func (m Model) Markdown() string {
	t, err := template.New("model").Parse(string(TemplateModelMarkdown))
	if err != nil {
		panic(err)
	}
	w := bytes.NewBuffer([]byte{})
	err = t.Execute(w, m)
	if err != nil {
		panic(err)
	}
	return w.String()
}

// Questionnaire returns some questions to guide the designer to ensure that
// the claimant model is sound.
func (m Model) Questionnaire() string {
	t, err := template.New("questions").Parse(string(TemplateQuestionsMarkdown))
	if err != nil {
		panic(err)
	}
	w := bytes.NewBuffer([]byte{})
	err = t.Execute(w, m)
	if err != nil {
		panic(err)
	}
	return w.String()
}

// ClaimTerms finds all of the terms used in the Claim that must be
// present in the Statement.
func (m Model) ClaimTerms() []string {
	re := regexp.MustCompile(`\$[\w\@]*`)

	return re.FindAllString(m.ClaimMarkdown(), -1)
}

// ClaimMarkdown renders the Claim(s) in markdown.
func (m Model) ClaimMarkdown() string {
	if len(m.Claims) == 0 {
		return m.Claim.Claim
	}
	r := "<ol>"
	for _, c := range m.Claims {
		r += fmt.Sprintf("<li>%s</li>", c.Claim)
	}
	r += "</ol>"
	return r
}

// VerifierList returns all of the verifiers mapped to the claim they verify.
func (m Model) VerifierList() map[string]string {
	if len(m.Claim.Verifier) > 0 {
		return map[string]string{m.Claim.Verifier: m.Claim.Claim}
	}
	r := make(map[string]string)
	for _, c := range m.Claims {
		r[c.Verifier] = c.Claim
	}
	return r
}

// VerifierMarkdown renders the Verifier(s) in markdown.
func (m Model) VerifierMarkdown() string {
	if len(m.Claims) == 0 {
		return fmt.Sprintf("%s: <i>%s</i>", m.Claim.Verifier, m.Claim.Claim)
	}
	r := "<ul>"
	for _, c := range m.Claims {
		r += fmt.Sprintf("<li>%s: <i>%s</i></li>", c.Verifier, c.Claim)
	}
	r += "</ul>"
	return r
}

// BelieverMarkdown renders the Believer(s) in markdown.
func (m Model) BelieverMarkdown() string {
	if len(m.Believer) > 0 {
		return m.Believer
	}
	r := "<ul>"
	for _, b := range m.Believers {
		r += fmt.Sprintf("<li>%s</li>", b)
	}
	r += "</ul>"
	return r
}

// LogModelForDomain proposes a template Claimant Model for human
// editing based on a domain model provided.
func LogModelForDomain(m Model) Model {
	verifiersString := m.Claim.Verifier
	if len(verifiersString) == 0 {
		verifiersString += "{"
		for _, c := range m.Claims {
			verifiersString += c.Verifier + "/"
		}
		verifiersString = verifiersString[:len(verifiersString)-1]
		verifiersString += "}"
	}
	believers := m.Believers
	if len(believers) == 0 {
		believers = append(believers, m.Believer)
	}
	if v := m.Claim.Verifier; len(v) > 0 {
		believers = append(believers, v)
	} else {
		for _, c := range m.Claims {
			believers = append(believers, c.Verifier)
		}
	}
	return Model{
		System:   fmt.Sprintf("LOG_%s", m.System),
		Claimant: fmt.Sprintf("TODO: %s/$LogOperator", m.Claimant),
		Claims: []Claim{
			{
				Claim:    "This data structure is append-only from any previous version",
				Verifier: "Witness",
			},
			{
				Claim:    "This data structure is globally consistent",
				Verifier: "Witness Quorum",
			},
			{
				Claim:    fmt.Sprintf("This data structure contains only leaves of type `%s`", m.Statement),
				Verifier: verifiersString,
			},
		},
		Statement: "Log Checkpoint",
		Believers: believers,
		Arbiter:   fmt.Sprintf("TODO: %s/$LogArbiter", m.Arbiter),
	}
}

// Models captures the domain model along with the log model that supports it.
// This can be extended for more general model composition in the future, but
// this is the most common composition and motiviation for Claimant Modelling.
type Models struct {
	Domain Model `yaml:"Domain"`
	Log    Model `yaml:"Log"`
}

// Actors returns all of the actors that participate in the ecosystem of logging
// the domain claims and verifying all behaviours.
func (ms Models) Actors() []string {
	am := make(map[string]bool)
	for _, model := range []Model{ms.Domain, ms.Log} {
		am[model.Claimant] = true
		for v := range model.VerifierList() {
			am[v] = true
		}
		if len(model.Believer) > 0 {
			am[model.Believer] = true
		} else {
			for _, b := range model.Believers {
				am[b] = true
			}
		}
		for v := range model.VerifierList() {
			am[v] = true
		}
	}
	r := make([]string, 0, len(am))
	for actor, _ := range am {
		if len(actor) > 0 {
			r = append(r, actor)
		}
	}
	// TODO(mhutchinson): put these in a more useful order than alphabetical
	sort.Strings(r)
	return r
}

// Markdown returns the markdown representation of both models.
func (ms Models) Markdown() string {
	return fmt.Sprintf("%s\n%s", ms.Domain.Markdown(), ms.Log.Markdown())
}

// SequenceDiagram returns a mermaid markdown snippet that shows the
// idealized workflow for this log ecosystem. This can be changed in the
// future to support other variations in the workflow, e.g. this generates
// a sequence that shows the claimant awaiting an inclusion proof and then
// creating an offline bundle, but not all ecosystems do this and so perhaps
// this should take some kind of Options that allows these cases to vary.
// For now, this is out of scope and this generated sequence diagram should
// be taken to represent the current best practice, and designers can modify
// it to reflect the deltas in their world.
func (ms Models) SequenceDiagram() string {
	t, err := template.New("seq").Parse(string(TemplateSequenceMarkdown))
	if err != nil {
		panic(err)
	}
	w := bytes.NewBuffer([]byte{})
	err = t.Execute(w, ms)
	if err != nil {
		panic(err)
	}
	return w.String()
}
