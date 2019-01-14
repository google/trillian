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

package testonly

import (
	"testing"

	"github.com/google/trillian/monitoring"
)

// TestCounter runs a test on a Counter produced from the provided MetricFactory.
func TestCounter(t *testing.T, factory monitoring.MetricFactory) {
	var tests = []struct {
		name       string
		labelNames []string
		labelVals  []string
	}{
		{
			name:       "counter0",
			labelNames: nil,
			labelVals:  nil,
		},
		{
			name:       "counter1",
			labelNames: []string{"key1"},
			labelVals:  []string{"val1"},
		},
		{
			name:       "counter2",
			labelNames: []string{"key1", "key2"},
			labelVals:  []string{"val1", "val2"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			counter := factory.NewCounter(test.name, "Test only", test.labelNames...)
			if got, want := counter.Value(test.labelVals...), 0.0; got != want {
				t.Errorf("Counter[%v].Value()=%v; want %v", test.labelVals, got, want)
			}
			counter.Inc(test.labelVals...)
			if got, want := counter.Value(test.labelVals...), 1.0; got != want {
				t.Errorf("Counter[%v].Value()=%v; want %v", test.labelVals, got, want)
			}
			counter.Add(2.5, test.labelVals...)
			if got, want := counter.Value(test.labelVals...), 3.5; got != want {
				t.Errorf("Counter[%v].Value()=%v; want %v", test.labelVals, got, want)
			}
			// Use an invalid number of labels.
			libels := append(test.labelVals, "bogus")
			counter.Add(10.0, libels...)
			counter.Inc(libels...)
			if got, want := counter.Value(libels...), 0.0; got != want {
				t.Errorf("Counter[%v].Value()=%v; want %v", libels, got, want)
			}
			// Check that the value hasn't changed.
			if got, want := counter.Value(test.labelVals...), 3.5; got != want {
				t.Errorf("Counter[%v].Value()=%v; want %v", test.labelVals, got, want)
			}
			// Use a different set of label values
			// Metrics with different valued label values, are distinct
			// This test is only applicable when a Metric has labels
			if test.labelVals != nil && len(test.labelVals) >= 1 {
				altLabels := make([]string, len(test.labelVals))
				copy(altLabels, test.labelVals)
				altLabels[0] = "alt-val1"
				// Increment counter using this different set of label values
				counter.Add(25.0, altLabels...)
				if got, want := counter.Value(altLabels...), 25.0; got != want {
					t.Errorf("Counter[%v].Value()=%v; want %v", altLabels, got, want)
				}
				// Counter with original set of label values should be unchanged
				if got, want := counter.Value(test.labelVals...), 3.5; got != want {
					t.Errorf("Counter[%v].Value()=%v; want %v", test.labelVals, got, want)
				}
			}
		})
	}
}

// TestGauge runs a test on a Gauge produced from the provided MetricFactory.
func TestGauge(t *testing.T, factory monitoring.MetricFactory) {
	var tests = []struct {
		name       string
		labelNames []string
		labelVals  []string
	}{
		{
			name:       "gauge0",
			labelNames: nil,
			labelVals:  nil,
		},
		{
			name:       "gauge1",
			labelNames: []string{"key1"},
			labelVals:  []string{"val1"},
		},
		{
			name:       "gauge2",
			labelNames: []string{"key1", "key2"},
			labelVals:  []string{"val1", "val2"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gauge := factory.NewGauge(test.name, "Test only", test.labelNames...)
			if got, want := gauge.Value(test.labelVals...), 0.0; got != want {
				t.Errorf("Gauge[%v].Value()=%v; want %v", test.labelVals, got, want)
			}
			gauge.Inc(test.labelVals...)
			if got, want := gauge.Value(test.labelVals...), 1.0; got != want {
				t.Errorf("Gauge[%v].Value()=%v; want %v", test.labelVals, got, want)
			}
			gauge.Dec(test.labelVals...)
			if got, want := gauge.Value(test.labelVals...), 0.0; got != want {
				t.Errorf("Gauge[%v].Value()=%v; want %v", test.labelVals, got, want)
			}
			gauge.Add(2.5, test.labelVals...)
			if got, want := gauge.Value(test.labelVals...), 2.5; got != want {
				t.Errorf("Gauge[%v].Value()=%v; want %v", test.labelVals, got, want)
			}
			gauge.Set(42.0, test.labelVals...)
			if got, want := gauge.Value(test.labelVals...), 42.0; got != want {
				t.Errorf("Gauge[%v].Value()=%v; want %v", test.labelVals, got, want)
			}
			// Use an invalid number of labels.
			libels := append(test.labelVals, "bogus")
			gauge.Add(10.0, libels...)
			gauge.Inc(libels...)
			gauge.Dec(libels...)
			gauge.Set(120.0, libels...)
			// Ask for an invalid number of labels.
			if got, want := gauge.Value(libels...), 0.0; got != want {
				t.Errorf("Gauge[%v].Value()=%v; want %v", libels, got, want)
			}
			// Check that the value hasn't changed.
			if got, want := gauge.Value(test.labelVals...), 42.0; got != want {
				t.Errorf("Gauge[%v].Value()=%v; want %v", test.labelVals, got, want)
			}
			// Use a different set of label values
			// Metrics with different valued label values, are distinct
			// This test is only applicable when a Metric has labels
			if test.labelVals != nil && len(test.labelVals) >= 1 {
				altLabels := make([]string, len(test.labelVals))
				copy(altLabels, test.labelVals)
				altLabels[0] = "alt-val1"
				// Set gauge using this different set of label values
				gauge.Set(25.0, altLabels...)
				if got, want := gauge.Value(altLabels...), 25.0; got != want {
					t.Errorf("Gauge[%v].Value()=%v; want %v", altLabels, got, want)
				}
				// Gauge with original set of label values should be unchanged
				if got, want := gauge.Value(test.labelVals...), 42.0; got != want {
					t.Errorf("Counter[%v].Value()=%v; want %v", test.labelVals, got, want)
				}
			}
		})
	}
}

// TestHistogram runs a test on a Histogram produced from the provided MetricFactory.
func TestHistogram(t *testing.T, factory monitoring.MetricFactory) {
	var tests = []struct {
		name       string
		labelNames []string
		labelVals  []string
	}{
		{
			name:       "histogram0",
			labelNames: nil,
			labelVals:  nil,
		},
		{
			name:       "histogram1",
			labelNames: []string{"key1"},
			labelVals:  []string{"val1"},
		},
		{
			name:       "histogram2",
			labelNames: []string{"key1", "key2"},
			labelVals:  []string{"val1", "val2"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			histogram := factory.NewHistogram(test.name, "Test only", test.labelNames...)
			gotCount, gotSum := histogram.Info(test.labelVals...)
			if wantCount, wantSum := uint64(0), 0.0; gotCount != wantCount || gotSum != wantSum {
				t.Errorf("Histogram[%v].Info()=%v,%v; want %v,%v", test.labelVals, gotCount, gotSum, wantCount, wantSum)
			}
			histogram.Observe(1.0, test.labelVals...)
			histogram.Observe(2.0, test.labelVals...)
			histogram.Observe(3.0, test.labelVals...)
			gotCount, gotSum = histogram.Info(test.labelVals...)
			if wantCount, wantSum := uint64(3), 6.0; gotCount != wantCount || gotSum != wantSum {
				t.Errorf("Histogram[%v].Info()=%v,%v; want %v,%v", test.labelVals, gotCount, gotSum, wantCount, wantSum)
			}

			// Use an invalid number of labels.
			libels := append(test.labelVals, "bogus")
			histogram.Observe(100.0, libels...)
			histogram.Observe(200.0, libels...)
			gotCount, gotSum = histogram.Info(libels...)
			if wantCount, wantSum := uint64(0), 0.0; gotCount != wantCount || gotSum != wantSum {
				t.Errorf("Histogram[%v].Info()=%v,%v; want %v,%v", libels, gotCount, gotSum, wantCount, wantSum)
			}
			// Check that the histogram hasn't changed.
			gotCount, gotSum = histogram.Info(test.labelVals...)
			if wantCount, wantSum := uint64(3), 6.0; gotCount != wantCount || gotSum != wantSum {
				t.Errorf("Histogram[%v].Info()=%v,%v; want %v,%v", test.labelVals, gotCount, gotSum, wantCount, wantSum)
			}
			// Use a different set of label values
			// Metrics with different valued label values, are distinct
			// This test is only applicable when a Metric has labels
			if test.labelVals != nil && len(test.labelVals) >= 1 {
				altLabels := make([]string, len(test.labelVals))
				copy(altLabels, test.labelVals)
				altLabels[0] = "alt-val1"
				// Observe histogram using this different set of label values
				histogram.Observe(25.0, altLabels...)
				histogram.Observe(50.0, altLabels...)
				gotCount, gotSum = histogram.Info(altLabels...)
				if wantCount, wantSum := uint64(2), 75.0; gotCount != wantCount || gotSum != wantSum {
					t.Errorf("Histogram[%v].Info()=%v,%v; want %v,%v", altLabels, gotCount, gotSum, wantCount, wantSum)
				}
				// Histogram with original set of label values should be unchanged
				gotCount, gotSum = histogram.Info(test.labelVals...)
				if wantCount, wantSum := uint64(3), 6.0; gotCount != wantCount || gotSum != wantSum {
					t.Errorf("Histogram[%v].Info()=%v,%v; want %v,%v", test.labelVals, gotCount, gotSum, wantCount, wantSum)
				}
			}
		})
	}
}
