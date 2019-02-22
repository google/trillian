package opencensus

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"contrib.go.opencensus.io/exporter/stackdriver"

	datadog "github.com/Datadog/opencensus-go-exporter-datadog"
	"github.com/golang/glog"
	"github.com/google/trillian/monitoring"
	multierror "github.com/hashicorp/go-multierror"

	"go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

const (
	namespace = "trillian"
	separator = "_"
)

// MetricFactory allows the creation of OpenCensus measures and views
type MetricFactory struct {
	Prefix string
}

// Initialize is called by Trillian Servers to configure user-specific Exporters.
// See https://github.com/google/trillian/pull/1414#pullrequestreview-195485927
// Once registered with Opencensus' View, all metrics will export to these systems
// Only Stackdriver requires a 60-second reporting period but this met be set once for all
// Stackdriver authentication uses Application Default Credentials and assumes
// GOOGLE_APPLICATION_CREDENTIALS references a service account key
// The service account permissions must include at least roles/monitoring.metricWriter
// Prometheus should expose a /metrics endpoint but currently does not
// Datadog exporter assumes the Datadog Agent is running on localhost
//TODO(dazwilkin) Is this the best place to initialize the exporters?
//TODO(dazwilkin) OpenCensus exporters should be configured by config
func Initialize() (func(), error) {
	var (
		err    error
		errors *multierror.Error
		dd     *datadog.Exporter
		pm     *prometheus.Exporter
		sd     *stackdriver.Exporter
	)

	// Stackdriver Exporter
	//TODO(dazwilkin) Make this dependent upon user-config
	if true {
		sd, err = stackdriver.NewExporter(stackdriver.Options{
			// MetricPrefix helps uniquely identify these metrics
			//TODO(dazwilkin) How to create the exporter in order to use MetricFactory.Prefix
			MetricPrefix: namespace,
		})
		errors = multierror.Append(errors, err)
		// Register Exporter
		view.RegisterExporter(sd)
		// Stackdriver requires 60s reporting period
		view.SetReportingPeriod(60 * time.Second)
		// Important to invoke Flush before exiting
		// Pushing 'defer sd.Flush()' to the returned function
	}

	// Prometheus Exporter
	// Provides an http.Handler for the metrics endpoint
	//TODO(dazwilkin) How to provide this handler back to the Trillian service?
	// e.g. mux.Handle("/metrics", pm)
	//TODO(dazwilkin) Make this dependent upon user-config
	if true {
		pm, err = prometheus.NewExporter(prometheus.Options{
			Namespace: namespace,
		})
		errors = multierror.Append(errors, err)
		// Register Exporter
		view.RegisterExporter(pm)
	}

	// Datadog Exporter
	// Assumes the Datadog agent is running
	//TODO(dazwilkin) Make this dependent upon user-config
	if true {
		dd, err = datadog.NewExporter(datadog.Options{
			Namespace: namespace,
		})
		errors = multierror.Append(errors, err)
		// Pushing 'defer dd.Stop()' to the returned function
		// Register Exporter
		view.RegisterExporter(dd)
	}

	return func() {
		//TODO(dazwilkin) Make this dependent upon user-config
		if true {
			dd.Stop()
		}
		// if prometheus {
		// 	// Nothing to do
		// }
		//TODO(dazwilkin) Make this dependent upon user-config
		if true {
			sd.Flush()
		}
	}, errors.ErrorOrNil()
}

// checkLabelNames as required by OpenCensus fails if any label name
// -- contains non-printable ASCII
// -- or len is 0 or >256
// Printable ASCII 32-126 inclusive
func checkLabelNames(names []string) {
	nonPrintableASCII := func(r rune) bool { return (r < 32 || r > 126) }
	for _, name := range names {
		if len(name) == 0 || len(name) > 256 {
			glog.Fatalf("OpenCensus label names must be between 1 and 256 characters")
		}
		if strings.IndexFunc(name, nonPrintableASCII) != -1 {
			glog.Fatalf("OpenCensus label names must be printable ASCII; '%s' is not", name)
		}
	}
}

// createTagKeys creates OpenCensus Tag Keys for each label name.
func createTagKeys(labelNames []string) []tag.Key {
	tagKeys := make([]tag.Key, len(labelNames))
	var err error
	for i, labelName := range labelNames {
		tagKeys[i], err = tag.NewKey(labelName)
		if err != nil {
			glog.Fatal(err)
		}
	}
	return tagKeys
}

// createMeasureAndView creates the OpenCensus Measure used to record stats and a View for reporting them
// Measurements are made against the Measure (returned)
// These are reported against any Views created with the Measure but, once registered, a handle to the view is dropped
func createMeasureAndView(prefix, name, help string, aggregation *view.Aggregation, labelNames []string) *stats.Float64Measure {
	if len(labelNames) >= 1 {
		// OpenCensus requires labelNames be (printable) ASCII
		checkLabelNames(labelNames)
	}

	//TODO(dazwilkin) Should the measure name be prefixed with the namespace?
	prefixedName := prefix + separator + name
	measure := stats.Float64(prefixedName, help, "1")
	tagKeys := createTagKeys(labelNames)

	v := &view.View{
		Name:        prefixedName,
		Measure:     measure,
		Description: help,
		Aggregation: aggregation,
		TagKeys:     tagKeys,
	}
	if err := view.Register(v); err != nil {
		log.Fatal(err)
	}

	return measure
}

// forAllLabelsAValue trivially ensures the numbers of labels matches the number of values
func forAllLabelsAValue(labels, values []string) error {
	if len(labels) != len(values) {
		return fmt.Errorf("Mismatched number of labels (%v) and values (%v)", len(labels), len(values))
	}
	return nil
}

// assignValuesToLabels creates OpenCensus tags (label=value pairs) for all labels
func assignValuesToLabels(ctx context.Context, labels, values []string) context.Context {
	for i, value := range values {
		// NewKey is idempotent and provides the Key so that we can insert its value
		label := labels[i]
		t, err := tag.NewKey(label)
		if err != nil {
			glog.Fatalf("Label [%s] not found", label)
		}
		ctx, _ = tag.New(ctx, tag.Insert(t, value))
	}
	return ctx
}

// NewCounter create a new Counter object backed by OpenCensus
func (ocmf MetricFactory) NewCounter(name, help string, labelNames ...string) monitoring.Counter {
	//TODO(dazwilkin) What View Aggregation is best for "Counter"? (sum?)
	glog.Infof("[Counter] %s", name)
	measure := createMeasureAndView(ocmf.Prefix, name, help, view.Sum(), labelNames)
	return &Counter{
		labelNames: labelNames,
		measure:    measure,
	}
}

// NewGauge creates a new Gauge object backed by OpenCensus
func (ocmf MetricFactory) NewGauge(name, help string, labelNames ...string) monitoring.Gauge {
	//TODO(dazwilkin) What View Aggregation is best for "Gauge"? (count+sum? lastvalue?)
	glog.Infof("[Gauge] %s", name)
	measure := createMeasureAndView(ocmf.Prefix, name, help, view.Sum(), labelNames)
	return &Gauge{
		labelNames: labelNames,
		measure:    measure,
	}
}

// Ref: https://github.com/google/trillian/blob/master/monitoring/prometheus/metrics.go
// buckets returns a reasonable range of histogram upper limits for most
// latency-in-seconds usecases.
func buckets() []float64 {
	// These parameters give an exponential range from 0.04 seconds to ~1 day.
	num := 300
	b := 1.05
	scale := 0.04

	r := make([]float64, 0, num)
	for i := 0; i < num; i++ {
		r = append(r, math.Pow(b, float64(i))*scale)
	}
	return r
}

// NewHistogram creates a new Histogram object backed by Prometheus.
func (ocmf MetricFactory) NewHistogram(name, help string, labelNames ...string) monitoring.Histogram {
	//TODO(dazwilkin) How is an OpenCensus Distribution treated by Stackdriver?
	glog.Infof("[Histogram] %s", name)
	measure := createMeasureAndView(ocmf.Prefix, name, help, view.Distribution(buckets()...), labelNames)
	return &Histogram{
		labelNames: labelNames,
		measure:    measure,
	}
}

// Counter is a wrapper around OpenCensus object
type Counter struct {
	labelNames []string
	measure    *stats.Float64Measure
}

// Inc adds 1 to a counter.
func (c *Counter) Inc(labelVals ...string) {
	c.Add(1.0, labelVals...)
}

// Add adds the given amount to a counter.
func (c *Counter) Add(val float64, labelVals ...string) {
	//TODO(dazwilkin) Are negative values permitted? Think not.
	// Nothing to do
	if val <= 0.0 {
		return
	}
	if err := forAllLabelsAValue(c.labelNames, labelVals); err != nil {
		glog.Error(err.Error())
		return
	}
	ctx := context.TODO()
	ctx = assignValuesToLabels(ctx, c.labelNames, labelVals)
	stats.Record(ctx, c.measure.M(val))
}

// Value returns the amount of a counter.
func (c *Counter) Value(labelVals ...string) float64 {
	if err := forAllLabelsAValue(c.labelNames, labelVals); err != nil {
		glog.Error(err.Error())
		return 0.0
	}
	glog.Error("Unable to return values for counters")
	return 0.0
}

// Gauge is a wrapper around an OpenCensus measurement and view
type Gauge struct {
	labelNames []string
	measure    *stats.Float64Measure
}

// Inc adds 1 to the gauge
func (g *Gauge) Inc(labelVals ...string) {
	glog.Error("Unable to increment gauge values; need to know the current value but don't")
	// g.Set(1.0, labelVals...)
}

// Dec subtracts 1 from the gauge
func (g *Gauge) Dec(labelVals ...string) {
	glog.Error("Unable to decrement gauge values; need to know the current value but don't")
	// g.Set(g.value-1.0, labelVals...)
}

// Add adds given value to the gauge
func (g *Gauge) Add(val float64, labelVals ...string) {
	glog.Error("Unable to add to gauge values; need to know the current value but don't.")
	// g.Set(val, labelVals...)
}

// Set sets the value of the gauge
func (g *Gauge) Set(val float64, labelVals ...string) {
	if err := forAllLabelsAValue(g.labelNames, labelVals); err != nil {
		glog.Error(err.Error())
		return
	}
	ctx := context.TODO()
	ctx = assignValuesToLabels(ctx, g.labelNames, labelVals)
	stats.Record(ctx, g.measure.M(val))
}

// Value returns the value of the gauge
func (g *Gauge) Value(labelVals ...string) float64 {
	if err := forAllLabelsAValue(g.labelNames, labelVals); err != nil {
		glog.Error(err.Error())
		return 0.0
	}
	glog.Error("Unable to return values for gauges")
	return 0.0
}

// Histogram is a wrapper around OpenCensus measurement
type Histogram struct {
	labelNames []string
	measure    *stats.Float64Measure
}

// Observe records a measure
func (h *Histogram) Observe(val float64, labelVals ...string) {
	if err := forAllLabelsAValue(h.labelNames, labelVals); err != nil {
		glog.Error(err.Error())
		return
	}
	ctx := context.TODO()
	ctx = assignValuesToLabels(ctx, h.labelNames, labelVals)
	stats.Record(ctx, h.measure.M(val))
}

// Info returns the count and sum of observations in the histogram
//TODO(dazwilkin) Unsure how to implement this for OpenCensus
func (h *Histogram) Info(labelVals ...string) (uint64, float64) {
	if err := forAllLabelsAValue(h.labelNames, labelVals); err != nil {
		glog.Error(err.Error())
		return 0, 0.0
	}
	glog.Error("Unable to return values for histograms")
	return 0, 0.0
}
