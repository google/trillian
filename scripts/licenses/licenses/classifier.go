// Copyright 2019 Google Inc. All Rights Reserved.
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

package licenses

import (
	"fmt"
	"io/ioutil"

	"github.com/google/licenseclassifier"
)

// Type identifies a class of software license.
type Type int

// License types
const (
	// Unknown type of license.
	Unknown = Type(iota)
	// Restricted licenses require mandatory source distribution if we ship a
	// product that includes third-party code protected by such a license.
	Restricted
	// Reciprocal licenses allow usage of software made available under such
	// licenses freely in *unmodified* form. If the third-party source code is
	// modified in any way these modifications to the original third-party
	// source code must be made available.
	Reciprocal
	// Notice licenses contain few restrictions, allowing original or modified
	// third-party software to be shipped in any product without endangering or
	// encumbering our source code. All of the licenses in this category do,
	// however, have an "original Copyright notice" or "advertising clause",
	// wherein any external distributions must include the notice or clause
	// specified in the license.
	Notice
	// Permissive licenses are even more lenient than a 'notice' license.
	// Not even a copyright notice is required for license compliance.
	Permissive
	// Unencumbered covers licenses that basically declare that the code is "free for any use".
	Unencumbered
	// Forbidden licenses are forbidden to be used.
	Forbidden
)

// Classifier can detect the type of a software license.
type Classifier struct {
	classifier *licenseclassifier.License
}

// NewClassifier creates a classifier that requires a specified confidence threshold
// in order to return a positive license classification.
func NewClassifier(confidenceThreshold float64) (*Classifier, error) {
	c, err := licenseclassifier.New(confidenceThreshold)
	if err != nil {
		return nil, err
	}
	return &Classifier{
		classifier: c,
	}, nil
}

// Identify returns the name of a license, given its file path.
func (c *Classifier) Identify(licensePath string) (string, error) {
	content, err := ioutil.ReadFile(licensePath)
	if err != nil {
		return "", err
	}
	matches := c.classifier.MultipleMatch(string(content), true)
	if len(matches) == 0 {
		return "", fmt.Errorf("unknown license")
	}
	return matches[0].Name, nil
}
