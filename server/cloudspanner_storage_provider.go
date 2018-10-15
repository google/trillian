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

package server

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
	"github.com/google/trillian/storage/cloudspanner"
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
	csDequeueAcrossMerkleBucketsFraction = flag.Float64("cloudspanner_dequeue_bucket_fraction", 0.75, "Fraction of merkle keyspace to dequeue from, set to zero to disable.")
	csReadOnlyStaleness                  = flag.Duration("cloudspanner_readonly_staleness", time.Minute, "How far in the past to perform readonly operations. Within limits, raising this should help to increase performance/reduce latency.")

	csMu              sync.RWMutex
	csStorageInstance *cloudSpannerProvider
)

func init() {
	if err := RegisterStorageProvider("cloud_spanner", newCloudSpannerStorageProvider); err != nil {
		panic(err)
	}
}

func warn() {
	w := `H4sIAAAAAAAA/4xUsW7rMAzc8xUE2lE41B2sWlzsZ3TwoKEIgkQZ64mLsxga8vUPlG3FrZ2Hd1Ng3onHE5UDPQOEmVnwjCGhjyLC8RLcPgfhIvmwot8/CaHF9CMdOthdGmKvdSQQET85TqxJtKzjgnd4mYaFilDIlmhsnKql977mZSqzYcLy5K/zCUX66sbtNOAwteTXiVph5m4nigGzzUH7e3+a3XIRf5PFyhQQEV6UXLeY8nL292gyujlMIlIbdcUwet9ieBx/snWIOXkievPenyOMiDjnjOHj+MMJhjZfFBFpHF+AcQkGpr9f1nz02YoKcPXed5nvjHG2DGtB//7gGwHCq69HIPMBGa7hIYi3mPlOBOhf/Z8eMBmAdNVjZKlCFuiQgK19Y1YKrXDT5KWX7ohVC+cArnUKwGAF/rwvk6CrVhJ1DuDDF9igfVtEuFf8U2M0MXW4wf28pBy/4yOuOaLZw2+Qa76m5PpSFy+5N0usbnyr66+AjY7cx3eKz5VHrZpFlqL6nJa82+gI/H3Vh+TKm9Fmib7I5GXSvcStTQrndxwIw4dQvpak00DGpKvbnVgIxXk4kD31oLnTSkgkxchmJ01Vnj7lQLZFXrV532bpfqLJbTzqfygXrLHkh/CoP5Hq13DXJYuV3fD/DcRbm+5f7s1tvNj/RLBD9T6vNbi9dYpT05QTKsV1B+Ut4m8AAAD///IJ0vhIBgAA`

	wd, _ := base64.StdEncoding.DecodeString(w)
	b := bytes.NewReader(wd)
	r, _ := gzip.NewReader(b)
	r.Close()
	t, _ := ioutil.ReadAll(r)
	glog.Warningf("WARNING\n%s\nCloudspanner is an experimental storage implementation, and only supports Logs currently.", string(t))
}

type cloudSpannerProvider struct {
	client *spanner.Client
}

func configFromFlags() spanner.ClientConfig {
	r := spanner.ClientConfig{}
	setIntIfNotDefault(&r.NumChannels, *csNumChannels)
	setUint64IfNotDefault(&r.SessionPoolConfig.MaxOpened, *csSessionMaxOpened)
	setUint64IfNotDefault(&r.SessionPoolConfig.MinOpened, *csSessionMinOpened)
	setUint64IfNotDefault(&r.SessionPoolConfig.MaxIdle, *csSessionMaxIdle)
	setUint64IfNotDefault(&r.SessionPoolConfig.MaxBurst, *csSessionMaxBurst)
	setFloat64IfNotDefault(&r.SessionPoolConfig.WriteSessions, *csSessionWriteSessions)
	setIntIfNotDefault(&r.SessionPoolConfig.HealthCheckWorkers, *csSessionHCWorkers)
	r.SessionPoolConfig.HealthCheckInterval = *csSessionHCInterval
	return r
}

func newCloudSpannerStorageProvider(mf monitoring.MetricFactory) (StorageProvider, error) {
	csMu.Lock()
	defer csMu.Unlock()

	if csStorageInstance != nil {
		return csStorageInstance, nil
	}

	client, err := spanner.NewClientWithConfig(context.TODO(), *csURI, configFromFlags())
	if err != nil {
		return nil, err
	}
	csStorageInstance = &cloudSpannerProvider{
		client: client,
	}
	return csStorageInstance, nil
}

func (s *cloudSpannerProvider) LogStorage() storage.LogStorage {
	warn()
	opts := cloudspanner.LogStorageOptions{}
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
	return cloudspanner.NewLogStorageWithOpts(s.client, opts)
}

func (s *cloudSpannerProvider) MapStorage() storage.MapStorage {
	warn()
	opts := cloudspanner.MapStorageOptions{}
	if *csReadOnlyStaleness > 0 {
		opts.ReadOnlyStaleness = *csReadOnlyStaleness
	}
	return cloudspanner.NewMapStorageWithOpts(s.client, opts)
}

func (s *cloudSpannerProvider) AdminStorage() storage.AdminStorage {
	warn()
	return cloudspanner.NewAdminStorage(s.client)
}

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
