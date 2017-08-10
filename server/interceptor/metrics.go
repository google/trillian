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

package interceptor

import (
	"fmt"
	"sync"

	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/quota"
)

const (
	badInfoReason            = "bad_info"
	badTreeReason            = "bad_tree"
	insufficientTokensReason = "insufficient_tokens"
)

var (
	requestCounter       monitoring.Counter
	requestDeniedCounter monitoring.Counter

	metricsOnce = sync.Once{}
)

func initMetrics(mf monitoring.MetricFactory) {
	metricsOnce.Do(func() {
		if mf == nil {
			mf = monitoring.InertMetricFactory{}
		}
		quota.InitMetrics(mf)
		requestCounter = mf.NewCounter("interceptor_request_count", "Total number of intercepted requests")
		requestDeniedCounter = mf.NewCounter(
			"interceptor_request_denied_count",
			"Number of requests by denied, labeled according to the reason for denial",
			"reason", monitoring.TreeIDLabel, "quota_user")
	})
}

func incRequestCounter() {
	requestCounter.Inc()
}

func incRequestDeniedCounter(reason string, treeID int64, quotaUser string) {
	requestDeniedCounter.Inc(reason, fmt.Sprint(treeID), quotaUser)
}
