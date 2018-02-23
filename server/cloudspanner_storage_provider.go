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
	"context"
	"flag"
	"sync"

	"cloud.google.com/go/spanner"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cloudspanner"
)

var (
	csURI                  = flag.String("cloudspanner_uri", "", "Connection URI for CloudSpanner database")
	csNumChannels          = flag.Int("cloudspanner_num_channels", 0, "Number of gRPC channels to use to talk to CloudSpanner.")
	csSessionMaxOpened     = flag.Uint64("cloudspanner_max_open_sessions", 0, "Max open sessions.")
	csSessionMinOpened     = flag.Uint64("cloudspanner_min_open_sessions", 0, "Min open sessions.")
	csSessionMaxIdle       = flag.Uint64("cloudspanner_max_idle_sessions", 0, "Max idle sessions.")
	csSessionMaxBurst      = flag.Uint64("cloudspanner_max_burst_sessions", 0, "Max concurrent create session requests.")
	csSessionWriteSessions = flag.Float64("cloudspanner_write_sessions", 0, "Fraction of write capable sessions to maintain.")
	csSessionHCWorkers     = flag.Int("cloudspanner_num_healthcheckers", 0, "Number of health check workers for Spanner session pool.")
	csSessionHCInterval    = flag.Duration("cloudspanner_healthcheck_interval", 0, "Interval betweek pinging sessions.")

	csMu              sync.RWMutex
	csStorageInstance *cloudSpannerProvider
)

func init() {
	if err := RegisterStorageProvider("cloud_spanner", newCloudSpannerStorageProvider); err != nil {
		panic(err)
	}
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
	return cloudspanner.NewLogStorage(s.client)
}

func (s *cloudSpannerProvider) MapStorage() storage.MapStorage {
	return nil
}

func (s *cloudSpannerProvider) AdminStorage() storage.AdminStorage {
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
