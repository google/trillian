// Copyright 2017 Google Inc. All Rights Reserved.
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

// Package signer is a sample implementation of a commit-log based signer.
package signer

import (
	"flag"
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/docs/storage/commit_log/simelection"
	"github.com/google/trillian/docs/storage/commit_log/simkafka"
)

var batchSize = flag.Int("batch_size", 5, "Maximum leaves to sign in one run")
var pessimizeInterval = flag.Duration("signer_pessimize", 10*time.Millisecond, "Pause interval in signing to induce inter-signer problems")

// Signer is a simulated signer instance.
type Signer struct {
	mu        sync.RWMutex
	Name      string
	election  *simelection.Election
	epoch     int64
	dbSTHInfo STHInfo
	db        FakeDatabase
}

// New creates a simulated signer that uses the provided election.
func New(name string, election *simelection.Election, epoch int64) *Signer {
	return &Signer{
		Name:     name,
		election: election,
		epoch:    epoch,
		dbSTHInfo: STHInfo{
			treeRevision: -1,
			sthOffset:    -1,
		},
	}
}

func (s *Signer) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefix := "  "
	if s.IsMaster() {
		prefix = "**"
	}
	return fmt.Sprintf("%s Signer %s up to STH{offset=%d, rev=%d} = %s\n", prefix, s.Name, s.dbSTHInfo.sthOffset, s.dbSTHInfo.treeRevision, s.dbSTHInfo.sth.String())
}

// LatestSTHInfo returns the most recent STHInfo known about by the signer
func (s *Signer) LatestSTHInfo() STHInfo {
	return s.dbSTHInfo
}

// StoreSTHInfo updates the STHInfo known about by the signer.
func (s *Signer) StoreSTHInfo(info STHInfo) {
	s.dbSTHInfo = info
}

// IsMaster indicates if this signer is master.
func (s *Signer) IsMaster() bool {
	if s.election == nil {
		return true
	}
	return s.election.IsMaster(s.Name)
}

// Run performs a single signing run.
func (s *Signer) Run() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Read from local DB to see what STH we know about locally.
	dbSTHInfo := s.LatestSTHInfo()
	glog.V(2).Infof("%s: our DB has data upto STH at %d", s.Name, dbSTHInfo.sthOffset)

	// Sanity check that the STH table has what we already know.
	if dbSTHInfo.sth.TreeSize > 0 {
		ourSTH := sthFromString(simkafka.Read("STHs/<treeID>", dbSTHInfo.sthOffset))
		if ourSTH == nil {
			glog.Errorf("%s: local DB has data ahead of STHs topic!!", s.Name)
			return
		}
		if ourSTH.Offset != dbSTHInfo.sthOffset {
			glog.Errorf("%s: local DB recorded offset %d but that has inconsistent STH %s!!", s.Name, dbSTHInfo.sthOffset, ourSTH)
			return
		}
		if ourSTH.TimeStamp != dbSTHInfo.sth.TimeStamp || ourSTH.TreeSize != dbSTHInfo.sth.TreeSize {
			glog.Errorf("%s: local DB has different data than STHs topic!!", s.Name)
			return
		}
		glog.V(2).Infof("%s: our DB at %v, matches STH at that offset", s.Name, dbSTHInfo.sthOffset)
	}

	// Look to see if anyone else has already stored data just ahead of our STH.  This will
	// normally be the next entry, but we need to ignore any entries that have inconsistent
	// offsets.
	nextOffset := dbSTHInfo.sthOffset
	var nextSTH *STH
	for {
		nextOffset++
		nextSTH = sthFromString(simkafka.Read("STHs/<treeID>", nextOffset))
		if nextSTH == nil {
			break
		}
		if nextSTH.Offset < nextOffset {
			// Found an entry in the STHs topic that didn't get stored at the offset its writer
			// expected it to be stored at, probably due to another master signer nipping in an
			// entry ahead of it (due to a bug in mastership election).
			// Kafka adjudicates the clash: whichever entry got the correct offset wins.
			glog.V(2).Infof("%s: ignoring inconsistent STH %s at offset %d", s.Name, nextSTH.String(), nextOffset)
			continue
		}
		if nextSTH.Offset > nextOffset {
			glog.Errorf("%s: STH %s is stored at offset %d, earlier than its writer expected!!", s.Name, nextSTH.String(), nextOffset)
			return
		}
		if nextSTH.TimeStamp < dbSTHInfo.sth.TimeStamp || nextSTH.TreeSize < dbSTHInfo.sth.TreeSize {
			glog.Errorf("%s: next STH %s has earlier timestamp than in local DB (%s)!!", s.Name, nextSTH.String(), dbSTHInfo.sth.String())
			return
		}
		break
	}

	time.Sleep(*pessimizeInterval)
	if nextSTH == nil {
		// We're up-to-date with the STHs topic (as of a moment ago) ...
		if !s.IsMaster() {
			glog.V(2).Infof("%s: up-to-date with STHs but not master, so exit", s.Name)
			return
		}
		// ... and we're the master. Move the STHs topic along to encompass any unincorporated leaves.
		offset := dbSTHInfo.sth.TreeSize
		batch := simkafka.ReadMultiple("Leaves/<treeID>", offset, *batchSize)
		glog.V(2).Infof("%s: nothing at next offset %d and we are master, so have read %d more leaves", s.Name, nextOffset, len(batch))
		if len(batch) == 0 {
			glog.V(2).Infof("%s: nothing to do", s.Name)
			return
		}
		timestamp := (time.Now().UnixNano() / int64(time.Millisecond)) - s.epoch
		newSTHInfo := STHInfo{
			sth: STH{
				TreeSize:  s.db.Size() + len(batch),
				TimeStamp: timestamp,
				Offset:    nextOffset, // The offset we expect this STH to end up at in STH topic
			},
			treeRevision: dbSTHInfo.treeRevision + 1,
		}
		newSTHInfo.sthOffset = simkafka.Append("STHs/<treeID>", newSTHInfo.sth.String())
		if newSTHInfo.sthOffset > nextOffset {
			// The STH didn't get stored at the offset we expected, presumably because someone else got there first
			glog.Warningf("%s: stored new STH %s at offset %d, which is unexpected; give up", s.Name, newSTHInfo.sth.String(), newSTHInfo.sthOffset)
			return
		}
		if newSTHInfo.sthOffset < nextOffset {
			glog.Errorf("%s: stored new STH %s at offset %d, which is earlier than expected!!", s.Name, newSTHInfo.sth.String(), newSTHInfo.sthOffset)
			return
		}
		glog.V(2).Infof("%s: stored new STH %s at expected offset, including %d new leaves", s.Name, newSTHInfo.sth.String(), len(batch))

		// Now the STH topic is updated (correctly), do our local DB
		s.db.AddLeaves(timestamp, nextOffset, batch)
		s.StoreSTHInfo(newSTHInfo)
	} else {
		// There is an STH one ahead of us that we're not caught up with yet.
		// Read the leaves between what we have in our DB, and that STH...
		count := nextSTH.TreeSize - dbSTHInfo.sth.TreeSize
		glog.V(2).Infof("%s: our DB is %d leaves behind the next STH at %s, so update it", s.Name, count, nextSTH.String())
		batch := simkafka.ReadMultiple("Leaves/<treeID>", dbSTHInfo.sth.TreeSize, count)
		if len(batch) != count {
			glog.Errorf("%s: expected to read leaves [%d, %d) but only got %d!!", s.Name, dbSTHInfo.sth.TreeSize, dbSTHInfo.sth.TreeSize+count, len(batch))
			return
		}
		// ... and store it in our local DB
		newSTHInfo := STHInfo{
			sth:          s.db.AddLeaves(nextSTH.TimeStamp, nextOffset, batch),
			treeRevision: dbSTHInfo.treeRevision + 1,
			sthOffset:    nextOffset,
		}
		glog.V(2).Infof("%s: update our DB to %s", s.Name, newSTHInfo.sth.String())
		s.StoreSTHInfo(newSTHInfo)
		// We may still not be caught up, but that's for the next time around.
	}
}
