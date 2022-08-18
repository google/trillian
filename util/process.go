// Copyright 2016 Google LLC. All Rights Reserved.
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

// Package util holds utility functions.
package util

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/klog/v2"
)

// AwaitSignal waits for standard termination signals, then runs the given
// function. Can early return if the passed in context is canceled, in which
// case the function is not run.
func AwaitSignal(ctx context.Context, doneFn func()) {
	// Subscribe for the standard set of signals used to terminate a server.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)

	// Wait for a signal or context cancellation.
	select {
	case sig := <-sigs:
		klog.Warningf("Signal received: %v", sig)
		doneFn()
	case <-ctx.Done():
		klog.Infof("AwaitSignal canceled: %v", ctx.Err())
	}
}
