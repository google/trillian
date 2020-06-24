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

// The commit_log binary runs a simulation of the design for a commit-log
// based signer, with a simulated Kafka-like interface and a simulated
// master election package (which can be triggered to incorrectly report
// multiple masters), and with the core algorithm in the signer code.
// glog.Warning is used throughout for unexpected-but-recoverable situations,
// whereas glog.Error is used for any situation that would indicate data
// corruption.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/docs/storage/commit_log/signer"
	"github.com/google/trillian/docs/storage/commit_log/simelection"
	"github.com/google/trillian/docs/storage/commit_log/simkafka"
)

var (
	runElections        = flag.Bool("run_elections", false, "Whether to use mastership election; if false, signers run in parallel")
	signerCount         = flag.Int("signer_count", 3, "Number of parallel signers to run")
	leafInterval        = flag.Duration("leaf_interval", 500*time.Millisecond, "Period between added leaves")
	eventInterval       = flag.Duration("event_interval", 1*time.Second, "Interval between events")
	masterChangePercent = flag.Int("master_change", 20, "Percent chance of a change of master")
	dualMasterPercent   = flag.Int("dual_master", 8, "Percent chance of a dual master")
	leafTogglePercent   = flag.Int("leaf_toggle", 10, "Percent chance of toggling leaf generation")
)

var names = []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}

func signerName(i int) string {
	if i < len(names) {
		return names[i]
	}
	return fmt.Sprintf("signer%d", i)
}

func increment(s string) string {
	if len(s) == 0 {
		return "A"
	}
	offset := len(s) - 1
	char := s[offset]
	var prefix string
	if len(s) > 1 {
		prefix = s[0:offset]
	}
	if char < 'Z' {
		char++
		return string(append([]byte(prefix), char))
	}
	return string(append([]byte(increment(prefix)), 'A'))
}

type lockedBool struct {
	mu  sync.RWMutex
	val bool
}

func (ab *lockedBool) Get() bool {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	return ab.val
}
func (ab *lockedBool) Set(v bool) {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	ab.val = v
}

func main() {
	flag.Parse()
	defer glog.Flush()

	epochMillis := time.Now().UnixNano() / int64(time.Millisecond)

	// Add leaves forever
	generateLeaves := lockedBool{val: true}
	go func() {
		nextLeaf := "A"
		for {
			time.Sleep(*leafInterval)
			if generateLeaves.Get() {
				simkafka.Append("Leaves/<treeID>", nextLeaf)
				nextLeaf = increment(nextLeaf)
			}
		}
	}()

	// Run a few signers forever
	var election *simelection.Election
	if *runElections {
		election = &simelection.Election{}
	} else {
		// Mastership manipulations are irrelevant if no elections.
		*masterChangePercent = 0
		*dualMasterPercent = 0
	}
	signers := []*signer.Signer{}
	for ii := 0; ii < *signerCount; ii++ {
		signers = append(signers, signer.New(signerName(ii), election, epochMillis))
	}
	for _, s := range signers {
		go func(s *signer.Signer) {
			for {
				time.Sleep(1 * time.Second)
				s.Run()
			}
		}(s)
	}

	for {
		choice := rand.Intn(100)
		switch {
		case choice < *masterChangePercent:
			which := rand.Intn(len(signers))
			who := signers[which].Name
			glog.V(1).Infof("EVENT: Move mastership from %v to [%v]", election.Masters(), who)
			election.SetMaster(who)
		case choice < (*masterChangePercent + *dualMasterPercent):
			if len(election.Masters()) > 1 {
				// Already in dual-master mode
				break
			}
			which1 := rand.Intn(len(signers))
			who1 := signers[which1].Name
			which2 := rand.Intn(len(signers))
			who2 := signers[which2].Name
			masters := []string{who1, who2}
			glog.V(1).Infof("EVENT: Make multiple mastership, from %v to %v", election.Masters(), masters)
			election.SetMasters(masters)
		case choice < (*masterChangePercent + *dualMasterPercent + *leafTogglePercent):
			val := generateLeaves.Get()
			glog.V(1).Infof("EVENT: Toggle leaf generation from %v to %v", val, !val)
			generateLeaves.Set(!val)
		}

		time.Sleep(*eventInterval)

		// Show current status
		output := simkafka.Status()
		for _, s := range signers {
			output += s.String()
		}
		fmt.Printf("\n%s\n", output)
	}
}
