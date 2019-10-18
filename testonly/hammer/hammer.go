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

package hammer

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/testonly"
)

const (
	defaultEmitSeconds = 10
	// How far beyond current revision to request for invalid requests
	invalidStretch = int64(10000)
	// rev=-1 is used when requesting the latest revision
	latestRevision = int64(-1)
	// Format specifier for generating leaf values
	valueFormat = "value-%09d"
	minValueLen = len("value-") + 9 // prefix + 9 digits
)

var (
	// Metrics are all per-map (label "mapid"), and per-entrypoint (label "ep").
	once        sync.Once
	reqs        monitoring.Counter   // mapid, ep => value
	errs        monitoring.Counter   // mapid, ep => value
	rsps        monitoring.Counter   // mapid, ep => value
	rspLatency  monitoring.Histogram // mapid, ep => distribution-of-values
	invalidReqs monitoring.Counter   // mapid, ep => value
)

// setupMetrics initializes all the exported metrics.
func setupMetrics(mf monitoring.MetricFactory) {
	reqs = mf.NewCounter("reqs", "Number of valid requests sent", "mapid", "ep")
	errs = mf.NewCounter("errs", "Number of error responses received for valid requests", "mapid", "ep")
	rsps = mf.NewCounter("rsps", "Number of responses received for valid requests", "mapid", "ep")
	rspLatency = mf.NewHistogram("rsp_latency", "Latency of responses received for valid requests in seconds", "mapid", "ep")
	invalidReqs = mf.NewCounter("invalid_reqs", "Number of deliberately-invalid requests sent", "mapid", "ep")
}

// errSkip indicates that a test operation should be skipped.
type errSkip struct{}

func (e errSkip) Error() string {
	return "test operation skipped"
}

// MapEntrypointName identifies a Map RPC entrypoint
type MapEntrypointName string

// Constants for entrypoint names, as exposed in statistics/logging.
const (
	GetLeavesName    = MapEntrypointName("GetLeaves")
	GetLeavesRevName = MapEntrypointName("GetLeavesRev")
	SetLeavesName    = MapEntrypointName("SetLeaves")
	GetSMRName       = MapEntrypointName("GetSMR")
	GetSMRRevName    = MapEntrypointName("GetSMRRev")
)

// Read-only map entry points.
var roMapEntrypoints = []MapEntrypointName{GetLeavesName, GetLeavesRevName, GetSMRName, GetSMRRevName}

// All map entry points.
var mapEntrypoints = append(roMapEntrypoints, SetLeavesName)

// Choice is a readable representation of a choice about how to perform a hammering operation.
type Choice string

// Constants for both valid and invalid operation choices.
const (
	ExistingKey    = Choice("ExistingKey")
	NonexistentKey = Choice("NonexistentKey")
	MalformedKey   = Choice("MalformedKey")
	DuplicateKey   = Choice("DuplicateKey")
	RevTooBig      = Choice("RevTooBig")
	RevIsNegative  = Choice("RevIsNegative")
	CreateLeaf     = Choice("CreateLeaf")
	UpdateLeaf     = Choice("UpdateLeaf")
	DeleteLeaf     = Choice("DeleteLeaf")
)

// MapBias indicates the bias for selecting different map operations.
type MapBias struct {
	Bias  map[MapEntrypointName]int
	total int
	// InvalidChance gives the odds of performing an invalid operation, as the N in 1-in-N.
	InvalidChance map[MapEntrypointName]int
}

// choose randomly picks an operation to perform according to the biases.
func (hb *MapBias) choose(r *rand.Rand) MapEntrypointName {
	if hb.total == 0 {
		for _, ep := range mapEntrypoints {
			hb.total += hb.Bias[ep]
		}
	}
	which := r.Intn(hb.total)
	for _, ep := range mapEntrypoints {
		which -= hb.Bias[ep]
		if which < 0 {
			return ep
		}
	}
	panic("random choice out of range")
}

// invalid randomly chooses whether an operation should be invalid.
func (hb *MapBias) invalid(ep MapEntrypointName, r *rand.Rand) bool {
	chance := hb.InvalidChance[ep]
	if chance <= 0 {
		return false
	}
	return r.Intn(chance) == 0
}

// MapConfig provides configuration for a stress/load test.
type MapConfig struct {
	MapID                int64 // 0 to use an ephemeral tree
	MetricFactory        monitoring.MetricFactory
	Client               trillian.TrillianMapClient
	Write                trillian.TrillianMapWriteClient
	Admin                trillian.TrillianAdminClient
	RandSource           rand.Source
	EPBias               MapBias
	LeafSize, ExtraSize  uint
	MinLeaves, MaxLeaves int
	Operations           uint64
	EmitInterval         time.Duration
	RetryErrors          bool
	OperationDeadline    time.Duration
	// NumCheckers indicates how many separate inclusion checker goroutines
	// to run.  Note that the behaviour of these checkers is not governed by
	// RandSource.
	NumCheckers int
	// KeepFailedTree indicates whether ephemeral trees should be left intact
	// after a failed hammer run.
	KeepFailedTree bool
}

// String conforms with Stringer for MapConfig.
func (c MapConfig) String() string {
	return fmt.Sprintf("mapID:%d biases:{%v} #operations:%d emit every:%v retryErrors? %t",
		c.MapID, c.EPBias, c.Operations, c.EmitInterval, c.RetryErrors)
}

// HitMap performs load/stress operations according to given config.
func HitMap(ctx context.Context, cfg MapConfig) error {
	var firstErr error

	if cfg.MapID == 0 {
		// No mapID provided, so create an ephemeral tree to test against.
		var err error
		cfg.MapID, err = makeNewMap(ctx, cfg.Admin, cfg.Client)
		if err != nil {
			return fmt.Errorf("failed to create ephemeral tree: %v", err)
		}
		glog.Infof("testing against ephemeral tree %d", cfg.MapID)
		defer func() {
			if firstErr != nil && cfg.KeepFailedTree {
				glog.Errorf("note: leaving ephemeral tree %d intact after error %v", cfg.MapID, firstErr)
				return
			}
			if err := destroyMap(ctx, cfg.Admin, cfg.MapID); err != nil {
				glog.Errorf("failed to destroy map with treeID %d: %v", cfg.MapID, err)
			}
		}()
	}

	s, err := newHammerState(ctx, &cfg)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(cfg.EmitInterval)
	go func(c <-chan time.Time) {
		for range c {
			glog.Info(s.String())
		}
	}(ticker.C)

	var wg sync.WaitGroup
	// Anything that arrives on errs terminates all processing (but there
	// may be more errors queued up behind it).
	errs := make(chan error, cfg.NumCheckers+1)
	// The done channel is used to signal all of the goroutines to
	// terminate.
	done := make(chan struct{})
	for i := 0; i < cfg.NumCheckers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := newReadWorker(s, i)
			glog.Infof("%d: start checker %d", s.cfg.MapID, i)
			err := w.run(ctx, done)
			if err != nil {
				errs <- err
			}
			glog.Infof("%d: checker %d done with %v", s.cfg.MapID, i, err)
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		w := newWriteWorker(s)
		glog.Infof("%d: start main goroutine", cfg.MapID)
		count, err := w.run(ctx, done)
		errs <- err // may be nil for the main goroutine completion
		glog.Infof("%d: performed %d operations on map", cfg.MapID, count)
	}()

	// Wait for first error, completion (which shows up as a nil error) or
	// external cancellation.
	select {
	case <-ctx.Done():
		glog.Infof("%d: context canceled", cfg.MapID)
	case e := <-errs:
		firstErr = e
		if firstErr != nil {
			glog.Infof("%d: first error encountered: %v", cfg.MapID, e)
		}
	}
	close(done)

	ticker.Stop()
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			glog.Infof("%d: error encountered: %v", cfg.MapID, e)
		}
	}
	// Emit final statistics
	glog.Info(s.String())
	return firstErr
}

// mapWorker represents a single entity in the Verifiable Map ecosystem.
// The worker may be a read-only client, or a writer which adds new entries to
// the map. Each worker should be as independent as possible (i.e. share little
// to no state), though through well defined interfaces this guideline may be
// ignored, which will allow effectively an in-memory gossip network to develop
// between workers, which makes the validation more significant.
//
// Each worker has its own PRNG, which makes the sequence of operations that it
// performs deterministic.
type mapWorker struct {
	prng  *rand.Rand
	mapID int64
	label string

	bias MapBias // Each worker can have its own customized map bias.

	retryErrors       bool
	operationDeadline time.Duration
}

func newWorker(cfg *MapConfig, bias MapBias, prng *rand.Rand) *mapWorker {
	return &mapWorker{
		prng:              prng,
		mapID:             cfg.MapID,
		label:             strconv.FormatInt(cfg.MapID, 10),
		bias:              bias,
		retryErrors:       cfg.RetryErrors,
		operationDeadline: cfg.OperationDeadline,
	}
}

func (w *mapWorker) retryOp(ctx context.Context, fn mapOperationFn, opName string) error {
	defer func(start time.Time) {
		rspLatency.Observe(time.Since(start).Seconds(), w.label, opName)
	}(time.Now())

	deadline := time.Now().Add(w.operationDeadline)
	seed := w.prng.Int63()
	done := false
	var firstErr error
	for !done {
		// Always re-create the same per-operation rand.Rand so any retries are exactly the same.
		prng := rand.New(rand.NewSource(seed))
		reqs.Inc(w.label, opName)
		err := fn(ctx, prng)

		switch err.(type) {
		case nil:
			rsps.Inc(w.label, opName)
			if firstErr != nil {
				glog.Warningf("%d: retry of op %v succeeded, previous error: %v", w.mapID, opName, firstErr)
			}
			firstErr = nil
			done = true
		case errSkip:
			firstErr = nil
			done = true
		case testonly.ErrInvariant:
			// Ensure invariant failures are not ignorable.  They indicate a design assumption
			// being broken or incorrect, so must be seen.
			firstErr = err
			done = true
		default:
			errs.Inc(w.label, opName)
			if firstErr == nil {
				firstErr = err
			}
			if w.retryErrors {
				glog.Warningf("%d: op %v failed (will retry): %v", w.mapID, opName, err)
			} else {
				done = true
			}
		}

		if time.Now().After(deadline) {
			if firstErr == nil {
				// If there was no other error, we've probably hit the deadline - make sure we bubble that up.
				firstErr = ctx.Err()
			}
			glog.Warningf("%d: gave up on operation %v after %v, returning first err %v", w.mapID, opName, w.operationDeadline, firstErr)
			done = true
		}
	}
	return firstErr
}

// readWorker performs read-only operations on a fixed map.
type readWorker struct {
	*mapWorker

	validReadOps   *validReadOps
	invalidReadOps *invalidReadOps
}

func newReadWorker(s *hammerState, idx int) *readWorker {
	readBias := MapBias{
		Bias:          make(map[MapEntrypointName]int),
		InvalidChance: make(map[MapEntrypointName]int),
	}
	for _, ep := range roMapEntrypoints {
		readBias.Bias[ep] = s.cfg.EPBias.Bias[ep]
		readBias.InvalidChance[ep] = s.cfg.EPBias.InvalidChance[ep]
	}
	return &readWorker{
		mapWorker: newWorker(s.cfg, readBias, rand.New(rand.NewSource(int64(idx)))),

		validReadOps:   s.validReadOps,
		invalidReadOps: s.invalidReadOps,
	}
}

// run continuously performs read-only operations against the map until the
// done channel is closed, or an error is encountered.
func (w *readWorker) run(ctx context.Context, done <-chan struct{}) error {
	for {
		select {
		case <-done:
			return nil
		default:
		}
		if err := w.readOnce(ctx); err != nil {
			if _, ok := err.(errSkip); ok {
				continue
			}
			return err
		}
	}
}

// TODO(mhutchinson): resolve duplication between this and retryOneOp.
func (w *readWorker) readOnce(ctx context.Context) error {
	ep := w.bias.choose(w.prng)
	if w.bias.invalid(ep, w.prng) {
		glog.V(3).Infof("%d: perform invalid %s operation", w.mapID, ep)
		invalidReqs.Inc(w.label, string(ep))
		op, err := getReadOp(ep, w.invalidReadOps)
		if err != nil {
			return err
		}
		return op(ctx, w.prng)
	}

	op, err := getReadOp(ep, w.validReadOps)
	if err != nil {
		return err
	}

	glog.V(3).Infof("%d: perform %s operation", w.mapID, ep)
	return w.retryOp(ctx, op, string(ep))
}

// writeWorker performs mutation operations on a fixed map.
type writeWorker struct {
	*mapWorker

	head       *testonly.MapContents
	operations uint64

	// TODO(mhutchinson): Remove hammerState from here - it allows access to global info
	// which makes reasoning about the behaviour difficult.
	s *hammerState

	// Counters for generating unique keys/values.
	keyIdx   int
	valueIdx int
}

func newWriteWorker(s *hammerState) *writeWorker {
	return &writeWorker{
		mapWorker: newWorker(s.cfg, s.cfg.EPBias, rand.New(s.cfg.RandSource)),

		operations: s.cfg.Operations,
		s:          s,
	}
}

// run continuously performs mutation operations on the map until the done channel is
// closed, an error is encountered, or the maximum number of operations have been performed.
func (w *writeWorker) run(ctx context.Context, done <-chan struct{}) (uint64, error) {
	count := uint64(0)

	for ; count < w.operations; count++ {
		select {
		case <-done:
			return count, nil
		default:
		}
		if err := w.writeOnce(ctx); err != nil {
			return count, err
		}
	}
	return count, nil
}

func (w *writeWorker) writeOnce(ctx context.Context) (err error) {
	ep := SetLeavesName
	if w.bias.invalid(ep, w.prng) {
		glog.V(3).Infof("%d: perform invalid %s operation", w.mapID, ep)
		invalidReqs.Inc(w.label, string(ep))
		return w.setLeavesInvalid(ctx, w.prng)
	}
	glog.V(3).Infof("%d: perform %s operation", w.mapID, ep)
	return w.retryOp(ctx, w.setLeaves, string(ep))
}

func (w *writeWorker) setLeaves(ctx context.Context, prng *rand.Rand) error {
	choices := []Choice{CreateLeaf, UpdateLeaf, DeleteLeaf}

	cfg := w.s.cfg
	n := pickIntInRange(cfg.MinLeaves, cfg.MaxLeaves, prng)
	if n == 0 {
		n = 1
	}
	leaves := make([]*trillian.MapLeaf, 0, n)
	rev := int64(0)
	if w.head != nil {
		rev = w.head.Rev
	}
leafloop:
	for i := 0; i < n; i++ {
		choice := choices[prng.Intn(len(choices))]
		if w.head.Empty() {
			choice = CreateLeaf
		}
		switch choice {
		case CreateLeaf:
			key := w.nextKey()
			value := w.nextValue()
			leaves = append(leaves, &trillian.MapLeaf{
				Index:     testonly.TransparentHash(key),
				LeafValue: value,
				ExtraData: testonly.ExtraDataForValue(value, cfg.ExtraSize),
			})
			glog.V(3).Infof("%d: %v: data[%q]=%q", w.mapID, choice, key, string(value))
		case UpdateLeaf, DeleteLeaf:
			key := w.head.PickKey(prng)
			// Not allowed to have the same key more than once in the same request
			for _, leaf := range leaves {
				if bytes.Equal(leaf.Index, key) {
					// Go back to the beginning of the loop and choose again.
					i--
					continue leafloop
				}
			}
			var value, extra []byte
			if choice == UpdateLeaf {
				value = w.nextValue()
				extra = testonly.ExtraDataForValue(value, cfg.ExtraSize)
			}
			leaves = append(leaves, &trillian.MapLeaf{Index: key, LeafValue: value, ExtraData: extra})
			glog.V(3).Infof("%d: %v: data[%q]=%q (extra=%q)", w.mapID, choice, dehash(key), string(value), string(extra))
		}
	}

	writeRev := uint64(rev + 1)

	req := trillian.WriteMapLeavesRequest{
		MapId:          w.mapID,
		Leaves:         leaves,
		Metadata:       metadataForRev(writeRev),
		ExpectRevision: int64(writeRev),
	}

	if err := w.s.sharedState.proposeLeaves(writeRev, leaves); err != nil {
		return err
	}
	if _, err := cfg.Write.WriteLeaves(ctx, &req); err != nil {
		return fmt.Errorf("failed to WriteLeaves(count=%d): %v", len(leaves), err)
	}

	glog.V(2).Infof("%d: set %d leaves, rev=%d", w.mapID, len(leaves), writeRev)
	w.head = w.head.UpdatedWith(writeRev, leaves)
	return nil
}

func (w *writeWorker) setLeavesInvalid(ctx context.Context, prng *rand.Rand) error {
	choices := []Choice{MalformedKey, DuplicateKey}

	var leaves []*trillian.MapLeaf
	value := []byte("value-for-invalid-req")

	choice := choices[prng.Intn(len(choices))]
	if w.head.Empty() {
		choice = MalformedKey
	}
	switch choice {
	case MalformedKey:
		key := testonly.TransparentHash("..invalid-size")
		leaves = append(leaves, &trillian.MapLeaf{Index: key[2:], LeafValue: value})
	case DuplicateKey:
		key := w.head.PickKey(prng)
		leaves = append(leaves, &trillian.MapLeaf{Index: key, LeafValue: value})
		leaves = append(leaves, &trillian.MapLeaf{Index: key, LeafValue: value})
	}
	req := trillian.WriteMapLeavesRequest{MapId: w.mapID, Leaves: leaves}
	rsp, err := w.s.cfg.Write.WriteLeaves(ctx, &req)
	if err == nil {
		return fmt.Errorf("unexpected success: set-leaves(%v: %+v): %+v", choice, req, rsp.Revision)
	}
	glog.V(2).Infof("%d: expected failure: set-leaves(%v: %+v): %+v", w.mapID, choice, req, rsp)
	return nil
}

func (w *writeWorker) nextKey() string {
	w.keyIdx++
	return fmt.Sprintf("key-%08d", w.keyIdx)
}

func (w *writeWorker) nextValue() []byte {
	w.valueIdx++
	result := make([]byte, w.s.cfg.LeafSize)
	copy(result, fmt.Sprintf(valueFormat, w.valueIdx))
	return result
}

// hammerState tracks the operations that have been performed during a test run.
type hammerState struct {
	cfg            *MapConfig
	validReadOps   *validReadOps
	invalidReadOps *invalidReadOps
	sharedState    *sharedState

	start time.Time
}

func newHammerState(ctx context.Context, cfg *MapConfig) (*hammerState, error) {
	tree, err := cfg.Admin.GetTree(ctx, &trillian.GetTreeRequest{TreeId: cfg.MapID})
	if err != nil {
		return nil, fmt.Errorf("failed to get tree information: %v", err)
	}
	glog.Infof("%d: hammering tree with configuration %+v", cfg.MapID, tree)
	mc, err := client.NewMapClientFromTree(cfg.Client, tree)
	if err != nil {
		return nil, fmt.Errorf("failed to get tree verifier: %v", err)
	}

	mf := cfg.MetricFactory
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}
	once.Do(func() { setupMetrics(mf) })
	if cfg.EmitInterval == 0 {
		cfg.EmitInterval = defaultEmitSeconds * time.Second
	}
	if cfg.MinLeaves < 0 {
		return nil, fmt.Errorf("invalid MinLeaves %d", cfg.MinLeaves)
	}
	if cfg.MaxLeaves < cfg.MinLeaves {
		return nil, fmt.Errorf("invalid MaxLeaves %d is less than MinLeaves %d", cfg.MaxLeaves, cfg.MinLeaves)
	}
	if int(cfg.LeafSize) < minValueLen {
		return nil, fmt.Errorf("invalid LeafSize %d is smaller than min %d", cfg.LeafSize, minValueLen)
	}
	if cfg.OperationDeadline == 0 {
		cfg.OperationDeadline = 60 * time.Second
	}

	sharedState := newSharedState()
	validReadOps := validReadOps{
		mc:           mc,
		extraSize:    cfg.ExtraSize,
		minLeaves:    cfg.MinLeaves,
		maxLeaves:    cfg.MaxLeaves,
		prevContents: sharedState.contents,
		sharedState:  sharedState,
	}
	invalidReadOps := invalidReadOps{
		mapID:        cfg.MapID,
		client:       cfg.Client,
		prevContents: sharedState.contents,
		sharedState:  sharedState,
	}

	return &hammerState{
		cfg:            cfg,
		start:          time.Now(),
		sharedState:    sharedState,
		validReadOps:   &validReadOps,
		invalidReadOps: &invalidReadOps,
	}, nil
}

func (s *hammerState) label() string {
	return strconv.FormatInt(s.cfg.MapID, 10)
}

func (s *hammerState) String() string {
	interval := time.Since(s.start)
	details := ""
	totalReqs := 0
	totalInvalidReqs := 0
	totalErrs := 0
	for _, ep := range mapEntrypoints {
		reqCount := int(reqs.Value(s.label(), string(ep)))
		totalReqs += reqCount
		if s.cfg.EPBias.Bias[ep] > 0 {
			details += fmt.Sprintf(" %s=%d/%d", ep, int(rsps.Value(s.label(), string(ep))), reqCount)
		}
		totalInvalidReqs += int(invalidReqs.Value(s.label(), string(ep)))
		totalErrs += int(errs.Value(s.label(), string(ep)))
	}
	revStr := "N/A"
	if latestRev, found := s.sharedState.getLastReadRev(); found {
		revStr = strconv.FormatUint(latestRev, 10)
	}
	return fmt.Sprintf("%d: lastSMR.rev=%v ops: total=%d (%f ops/sec) invalid=%d errs=%v%s", s.cfg.MapID, revStr, totalReqs, float64(totalReqs)/interval.Seconds(), totalInvalidReqs, totalErrs, details)
}

func pickIntInRange(min, max int, prng *rand.Rand) int {
	delta := 1 + max - min
	return min + prng.Intn(delta)
}

type readOps interface {
	getLeaves(context.Context, *rand.Rand) error
	getLeavesRev(context.Context, *rand.Rand) error
	getSMR(context.Context, *rand.Rand) error
	getSMRRev(context.Context, *rand.Rand) error
}

type mapOperationFn func(context.Context, *rand.Rand) error

func getReadOp(ep MapEntrypointName, read readOps) (mapOperationFn, error) {
	switch ep {
	case GetLeavesName:
		return read.getLeaves, nil
	case GetLeavesRevName:
		return read.getLeavesRev, nil
	case GetSMRName:
		return read.getSMR, nil
	case GetSMRRevName:
		return read.getSMRRev, nil
	default:
		return nil, fmt.Errorf("internal error: unknown read operation %s", ep)
	}
}

func dehash(index []byte) string {
	return strings.TrimRight(string(index), "\x00")
}

// metadataForRev returns the metadata value that the maphammer always uses for
// a specific revision.
func metadataForRev(rev uint64) []byte {
	if rev == 0 {
		return []byte{}
	}
	return []byte(fmt.Sprintf("Metadata-%d", rev))
}
