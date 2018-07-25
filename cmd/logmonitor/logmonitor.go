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

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/client"
	"github.com/jinzhu/gorm"
)

// LogMonitor is an object used to continuously monitor a Trillian log.
type LogMonitor struct {
	logID                int64
	logClient            *client.LogClient
	db                   *gorm.DB
	waitBetweenDBUpdates time.Duration
}

// Run starts continuously monitoring the Trillian log specified in the
// LogMonitor object.
//
// It continues running until it encounters an error.
func (m *LogMonitor) Run(ctx context.Context) error {
	go m.periodicallyPersist(ctx)
	for {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if err == context.Canceled {
				return nil
			}
			return err
		default:
		}

		// Wait for root updates.
		_, err := m.logClient.WaitForRootUpdate(ctx)
		if err != nil {
			return fmt.Errorf("root update failed: %v", err)
		}
		glog.Infof("Root updated for %d", m.logID)
	}
}

// periodicallyPersist periodically writes the latest root value to storage to
// gracefully handle restarts and outages.
func (m *LogMonitor) periodicallyPersist(ctx context.Context) {
	latestPersistedRoot := m.logClient.GetRoot()
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(m.waitBetweenDBUpdates):
			latestRoot := m.logClient.GetRoot()

			if latestRoot.TimestampNanos <= latestPersistedRoot.TimestampNanos &&
				latestRoot.TreeSize <= latestPersistedRoot.TreeSize {
				continue
			}

			glog.Infof("Saving new root for %d to the database", m.logID)
			if err := WriteRootFor(m.db, m.logID, latestRoot); err != nil {
				glog.Errorf("Failed to write the root update to db for log %d: %v", m.logID, err)
			}

			latestPersistedRoot = latestRoot
		}
	}
}
