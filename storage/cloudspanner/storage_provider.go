// Copyright 2018 Google LLC. All Rights Reserved.
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

package cloudspanner

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"flag"
	"io/ioutil"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/glog"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"google.golang.org/api/option"
)

var (
	csURI                                = flag.String("cloudspanner_uri", "", "Connection URI for CloudSpanner database")
	csNumChannels                        = flag.Int("cloudspanner_num_channels", 0, "Number of gRPC channels to use to talk to CloudSpanner.")
	csSessionMaxOpened                   = flag.Uint64("cloudspanner_max_open_sessions", 0, "Max open sessions.")
	csSessionMinOpened                   = flag.Uint64("cloudspanner_min_open_sessions", 0, "Min open sessions.")
	csSessionMaxIdle                     = flag.Uint64("cloudspanner_max_idle_sessions", 0, "Max idle sessions.")
	csSessionMaxBurst                    = flag.Uint64("cloudspanner_max_burst_sessions", 0, "Max concurrent create session requests.")
	csSessionWriteSessions               = flag.Float64("cloudspanner_write_sessions", 0, "Fraction of write capable sessions to maintain.")
	csSessionHCWorkers                   = flag.Int("cloudspanner_num_healthcheckers", 0, "Number of health check workers for Spanner session pool.")
	csSessionHCInterval                  = flag.Duration("cloudspanner_healthcheck_interval", 0, "Interval betweek pinging sessions.")
	csSessionTrackHandles                = flag.Bool("cloudspanner_track_session_handles", false, "determines whether the session pool will keep track of the stacktrace of the goroutines that take sessions from the pool.")
	csDequeueAcrossMerkleBucketsFraction = flag.Float64("cloudspanner_dequeue_bucket_fraction", 0.75, "Fraction of merkle keyspace to dequeue from, set to zero to disable.")
	csReadOnlyStaleness                  = flag.Duration("cloudspanner_readonly_staleness", time.Minute, "How far in the past to perform readonly operations. Within limits, raising this should help to increase performance/reduce latency.")

	csMu              sync.RWMutex
	csStorageInstance *cloudSpannerProvider
	warnOnce          sync.Once
)

func init() {
	if err := storage.RegisterProvider("cloud_spanner", newCloudSpannerStorageProvider); err != nil {
		panic(err)
	}
}

func warn() {
	warnOnce.Do(func() {
		w := `H4sIAAAAAAAA/4xUsW7rMAzc8xUE2lE41B2sWlzsZ3TwoKEIgkQZ64mLsxga8vUPlG3FrZ2Hd1Ng3onHE5UDPQOEmVnwjCGhjyLC8RLcPgfhIvmwot8/CaHF9CMdOthdGmKvdSQQET85TqxJtKzjgnd4mYaFilDIlmhsnKql977mZSqzYcLy5K/zCUX66sbtNOAwteTXiVph5m4nigGzzUH7e3+a3XIRf5PFyhQQEV6UXLeY8nL292gyujlMIlIbdcUwet9ieBx/snWIOXkievPenyOMiDjnjOHj+MMJhjZfFBFpHF+AcQkGpr9f1nz02YoKcPXed5nvjHG2DGtB//7gGwHCq69HIPMBGa7hIYi3mPlOBOhf/Z8eMBmAdNVjZKlCFuiQgK19Y1YKrXDT5KWX7ohVC+cArnUKwGAF/rwvk6CrVhJ1DuDDF9igfVtEuFf8U2M0MXW4wf28pBy/4yOuOaLZw2+Qa76m5PpSFy+5N0usbnyr66+AjY7cx3eKz5VHrZpFlqL6nJa82+gI/H3Vh+TKm9Fmib7I5GXSvcStTQrndxwIw4dQvpak00DGpKvbnVgIxXk4kD31oLnTSkgkxchmJ01Vnj7lQLZFXrV532bpfqLJbTzqfygXrLHkh/CoP5Hq13DXJYuV3fD/DcRbm+5f7s1tvNj/RLBD9T6vNbi9dYpT05QTKsV1B+Ut4m8AAAD///IJ0vhIBgAA`

		wd, _ := base64.StdEncoding.DecodeString(w)
		b := bytes.NewReader(wd)
		r, _ := gzip.NewReader(b)
		if err := r.Close(); err != nil {
			// No need to exit, it's an unlikely error and doesn't affect operation.
			glog.Warningf("Close()=%v", err)
		}
		t, _ := ioutil.ReadAll(r)
		glog.Warningf("WARNING\n%s\nCloudspanner is an experimental storage implementation, and only supports Logs currently.", string(t))
	})
}

type cloudSpannerProvider struct {
	client *spanner.Client
}

func configFromFlags() spanner.ClientConfig {
	r := spanner.ClientConfig{}
	setUint64IfNotDefault(&r.SessionPoolConfig.MaxOpened, *csSessionMaxOpened)
	setUint64IfNotDefault(&r.SessionPoolConfig.MinOpened, *csSessionMinOpened)
	setUint64IfNotDefault(&r.SessionPoolConfig.MaxIdle, *csSessionMaxIdle)
	setUint64IfNotDefault(&r.SessionPoolConfig.MaxBurst, *csSessionMaxBurst)
	setFloat64IfNotDefault(&r.SessionPoolConfig.WriteSessions, *csSessionWriteSessions)
	setIntIfNotDefault(&r.SessionPoolConfig.HealthCheckWorkers, *csSessionHCWorkers)
	r.SessionPoolConfig.TrackSessionHandles = *csSessionTrackHandles
	r.SessionPoolConfig.HealthCheckInterval = *csSessionHCInterval
	return r
}

func optionsFromFlags() []option.ClientOption {
	opts := []option.ClientOption{}
	if numConns := *csNumChannels; numConns != 0 {
		opts = append(opts, option.WithGRPCConnectionPool(numConns))
	}
	return opts
}

func newCloudSpannerStorageProvider(_ monitoring.MetricFactory) (storage.Provider, error) {
	csMu.Lock()
	defer csMu.Unlock()

	if csStorageInstance != nil {
		return csStorageInstance, nil
	}

	client, err := spanner.NewClientWithConfig(context.TODO(), *csURI, configFromFlags(), optionsFromFlags()...)
	if err != nil {
		return nil, err
	}
	csStorageInstance = &cloudSpannerProvider{
		client: client,
	}
	return csStorageInstance, nil
}

// LogStorage builds and returns a new storage.LogStorage using CloudSpanner.
func (s *cloudSpannerProvider) LogStorage() storage.LogStorage {
	warn()
	opts := LogStorageOptions{}
	frac := *csDequeueAcrossMerkleBucketsFraction
	if frac > 1.0 {
		frac = 1.0
	}
	if frac > 0 {
		opts.DequeueAcrossMerkleBuckets = true
		opts.DequeueAcrossMerkleBucketsRangeFraction = frac
	}
	if *csReadOnlyStaleness > 0 {
		opts.ReadOnlyStaleness = *csReadOnlyStaleness
	}
	return NewLogStorageWithOpts(s.client, opts)
}

// MapStorage builds and returns a new storage.MapStorage using CloudSpanner.
func (s *cloudSpannerProvider) MapStorage() storage.MapStorage {
	warn()
	opts := MapStorageOptions{}
	if *csReadOnlyStaleness > 0 {
		opts.ReadOnlyStaleness = *csReadOnlyStaleness
	}
	return NewMapStorageWithOpts(s.client, opts)
}

// AdminStorage builds and returns a new storage.AdminStorage using CloudSpanner.
func (s *cloudSpannerProvider) AdminStorage() storage.AdminStorage {
	warn()
	return NewAdminStorage(s.client)
}

// Close shuts down this provider. Calls to the other methods will fail
// after this.
func (s *cloudSpannerProvider) Close() error {
	s.client.Close()
	return nil
}

func setIntIfNotDefault(t *int, v int) {
	if v != 0 {
		*t = v
	}
}

func setUint64IfNotDefault(t *uint64, v uint64) {
	if v != 0 {
		*t = v
	}
}

func setFloat64IfNotDefault(t *float64, v float64) {
	if v != 0 {
		*t = v
	}
}
