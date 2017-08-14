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

package monitoring

var globalMF MetricFactory = &fwdMF{createChan: make(chan func(MetricFactory), 100)}

// MF returns the globally registered MetricFactory.
func MF() MetricFactory {
	return globalMF
}

// SetMF sets the global MetricFactory.
// The newly-assigned factory will be prompted to create all requested metrics so far.
func SetMF(mf MetricFactory) {
	if f, ok := globalMF.(*fwdMF); ok {
		go f.processCreates(mf)
	}
	globalMF = mf
}

// fwdMF plays the part of the global MF until a proper implementation is assigned (by calling
// SetMF).
// Proxy, noop metrics are created until SetMF() is called, at that point processCreates() is
// triggered and calls are forwarded to the underlying, "real" MetricFactory.
type fwdMF struct {
	createChan chan func(MetricFactory)
}

func (f *fwdMF) processCreates(mf MetricFactory) {
	for true {
		fn := <-f.createChan
		fn(mf)
	}
}

func (f *fwdMF) NewCounter(name, help string, labelNames ...string) Counter {
	c := &fwdCounter{}
	f.createChan <- func(mf MetricFactory) {
		c.impl = mf.NewCounter(name, help, labelNames...)
	}
	return c
}

func (f *fwdMF) NewGauge(name, help string, labelNames ...string) Gauge {
	g := &fwdGauge{}
	f.createChan <- func(mf MetricFactory) {
		g.impl = mf.NewGauge(name, help, labelNames...)
	}
	return g
}

func (f *fwdMF) NewHistogram(name, help string, labelNames ...string) Histogram {
	h := &fwdHistogram{}
	f.createChan <- func(mf MetricFactory) {
		h.impl = mf.NewHistogram(name, help, labelNames...)
	}
	return h
}

type fwdCounter struct {
	impl Counter
}

func (c *fwdCounter) Inc(labelVals ...string) {
	if c.impl != nil {
		c.impl.Inc(labelVals...)
	}
}

func (c *fwdCounter) Add(val float64, labelVals ...string) {
	if c.impl != nil {
		c.impl.Add(val, labelVals...)
	}
}

func (c *fwdCounter) Value(labelVals ...string) float64 {
	if c.impl != nil {
		return c.impl.Value(labelVals...)
	}
	return 0
}

type fwdGauge struct {
	impl Gauge
}

func (g *fwdGauge) Inc(labelVals ...string) {
	if g.impl != nil {
		g.Inc(labelVals...)
	}
}

func (g *fwdGauge) Dec(labelVals ...string) {
	if g.impl != nil {
		g.Dec(labelVals...)
	}
}

func (g *fwdGauge) Add(val float64, labelVals ...string) {
	if g.impl != nil {
		g.Add(val, labelVals...)
	}
}

func (g *fwdGauge) Set(val float64, labelVals ...string) {
	if g.impl != nil {
		g.Set(val, labelVals...)
	}
}

func (g *fwdGauge) Value(labelVals ...string) float64 {
	if g.impl != nil {
		g.Value(labelVals...)
	}
	return 0
}

type fwdHistogram struct {
	impl Histogram
}

func (h *fwdHistogram) Observe(val float64, labelVals ...string) {
	if h.impl != nil {
		h.impl.Observe(val, labelVals...)
	}
}

func (h *fwdHistogram) Info(labelVals ...string) (uint64, float64) {
	if h.impl != nil {
		return h.impl.Info(labelVals...)
	}
	return 0, 0
}
