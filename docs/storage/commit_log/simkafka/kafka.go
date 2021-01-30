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

// Package simkafka is a toy simulation of a Kafka commit log.
package simkafka

import (
	"fmt"
	"sync"
)

type commitLog []string

const showCount = 10

func (c commitLog) String() string {
	result := ""
	l := len(c)
	start := l - showCount
	if start < 0 {
		start = 0
	} else if start > 0 {
		result += "... "
	}
	for i := start; i < l; i++ {
		result += fmt.Sprintf("| %d:%s ", i, c[i])
	}
	result += "|"
	return result
}

var (
	mu     sync.RWMutex
	topics = make(map[string]commitLog)
)

// Status reports the current status of the simulated Kafka instance
func Status() string {
	mu.RLock()
	defer mu.RUnlock()
	result := ""
	for key, commit := range topics {
		result += fmt.Sprintf("%s: %s\n", key, commit)
	}
	return result
}

// Read returns a value for a topic at a specific offset.
func Read(which string, offset int) string {
	mu.RLock()
	defer mu.RUnlock()
	topic, ok := topics[which]
	if !ok {
		return ""
	}
	if offset >= len(topic) {
		return ""
	}
	return topic[offset]
}

// ReadLast returns the latest value for a topic, and its offset
func ReadLast(which string) (string, int) {
	mu.RLock()
	defer mu.RUnlock()
	topic, ok := topics[which]
	if !ok {
		return "", -1
	}
	offset := len(topic) - 1
	return topic[offset], offset
}

// ReadMultiple reads values for a topic, starting at the given offset, up to the
// given maximum number of results.
func ReadMultiple(which string, offset, max int) []string {
	mu.RLock()
	defer mu.RUnlock()
	topic, ok := topics[which]
	if !ok {
		return nil
	}
	if offset > len(topic) {
		return nil
	}
	if offset+max > len(topic) {
		max = len(topic) - offset
	}
	return topic[offset : offset+max]
}

// Append adds a value to the end of a topic, and returns the offset of the added value.
func Append(which string, what string) int {
	mu.Lock()
	defer mu.Unlock()
	topics[which] = append(topics[which], what)
	return len(topics[which]) - 1
}
