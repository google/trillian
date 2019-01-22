package opencensus

import (
	"testing"

	"github.com/google/trillian/monitoring/testonly"
)

// TODO: [dazwilkin:181206] Test for non-ASCII tag keys|values

func TestCounter(t *testing.T) {
	testonly.TestCounter(t, MetricFactory{Prefix: "TestCounter"})
}

func TestGauge(t *testing.T) {
	testonly.TestGauge(t, MetricFactory{Prefix: "TestGauge"})
}
func TestHistogram(t *testing.T) {
	testonly.TestHistogram(t, MetricFactory{Prefix: "TestHistogram"})
}
