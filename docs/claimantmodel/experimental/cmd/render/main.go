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

// render is a command to take a Claimant Model specified as yaml and
// output markdown representations of it.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	claimant "github.com/google/trillian/docs/claimantmodel/experimental/cmd/render/internal"
	"gopkg.in/yaml.v2"
	"k8s.io/klog/v2"
)

var (
	dmf = flag.String("domain_model_file", "", "path to domain model yaml file")
	fmf = flag.String("full_model_file", "", "path to full model yaml file")
)

func main() {
	flag.Parse()

	if (len(*dmf) == 0) == (len(*fmf) == 0) {
		klog.Exitf("--domain_model_file OR --full_model_file are required")
	}

	if len(*dmf) > 0 {
		saved, err := os.ReadFile(*dmf)
		if err != nil {
			klog.Exitf("failed to read model: %v", err)
		}

		domain := claimant.Model{}
		if err := yaml.Unmarshal(saved, &domain); err != nil {
			klog.Exitf("failed to parse Model: %v", err)
		}
		handleSingleModel(domain)
	} else {
		saved, err := os.ReadFile(*fmf)
		if err != nil {
			klog.Exitf("failed to read model: %v", err)
		}

		models := claimant.Models{}
		if err := yaml.Unmarshal(saved, &models); err != nil {
			klog.Exitf("failed to parse Models: %v", err)
		}
		handleMultiModels(models)
	}
}

func handleSingleModel(domain claimant.Model) {
	fmt.Printf("Domain Model as markdown:\n%s\n\n", domain.Markdown())

	models := claimant.Models{
		Domain: domain,
		Log:    claimant.LogModelForDomain(domain),
	}

	mbs, err := yaml.Marshal(models)
	if err != nil {
		klog.Exitf("failed to marshal models: %v", err)
	}
	fmt.Printf("Complete model template as yaml:\n%s\n\n", string(mbs))
}

func handleMultiModels(models claimant.Models) {
	generateCommand := getGenerateDocs()
	fmt.Printf("All actors:\n%s\n\n", strings.Join(models.Actors(), "\n"))
	fmt.Printf("Models as markdown:\n%s\n%s\n\n", generateCommand, models.Markdown())
	fmt.Printf("Sequence diagrams:\n%s\n%s\n\n", generateCommand, models.SequenceDiagram())
}

func getGenerateDocs() string {
	builder := strings.Builder{}
	builder.WriteString("<!--- This content generated with:\n")
	builder.WriteString("go run github.com/google/trillian/docs/claimantmodel/experimental/cmd/render@master")
	for _, a := range os.Args[1:] {
		builder.WriteString(fmt.Sprintf(" %s", a))
	}
	builder.WriteString("\n-->")
	return builder.String()
}
