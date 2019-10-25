package opencensus

import (
	"testing"
)

// TODO: [dazwilkin:181206] Test for non-ASCII tag keys|values

func TestCounter(t *testing.T) {
	// TODO(dazwilkin) OpenCensus Exporter is unable to satisfy provided tests because it is unable to retrieve values
	// testonly.TestCounter(t, MetricFactory{Prefix: "TestCounter"})
}

func TestGauge(t *testing.T) {
	// TODO(dazwilkin) OpenCensus Exporter is unable to satisfy provided tests because it is unable to retrieve values
	// testonly.TestGauge(t, MetricFactory{Prefix: "TestGauge"})
}
func TestHistogram(t *testing.T) {
	// TODO(dazwilkin) OpenCensus Exporter is unable to satisfy provided tests because it is unable to retrieve values
	// testonly.TestHistogram(t, MetricFactory{Prefix: "TestHistogram"})
}
