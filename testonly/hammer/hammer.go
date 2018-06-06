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
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
)

const (
	defaultEmitSeconds = 10
	// How many SMRs to hold on to.
	smrCount = 30
	// How far beyond current revision to request for invalid requests
	invalidStretch = int64(10000)
	// rev=-1 is used when requesting the latest revision
	latestRevision = int64(-1)
)

var (
	maxRetryDuration = 60 * time.Second
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

// errInvariant indicates that an invariant check failed, with details in msg.
type errInvariant struct {
	msg string
}

func (e errInvariant) Error() string {
	return fmt.Sprintf("Invariant check failed: %v", e.msg)
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

var mapEntrypoints = []MapEntrypointName{GetLeavesName, GetLeavesRevName, SetLeavesName, GetSMRName, GetSMRRevName}

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
func (hb *MapBias) choose(s rand.Source) MapEntrypointName {
	if hb.total == 0 {
		for _, ep := range mapEntrypoints {
			hb.total += hb.Bias[ep]
		}
	}
	which := intN(s, hb.total)
	for _, ep := range mapEntrypoints {
		which -= hb.Bias[ep]
		if which < 0 {
			return ep
		}
	}
	panic("random choice out of range")
}

// invalid randomly chooses whether an operation should be invalid.
func (hb *MapBias) invalid(ep MapEntrypointName, s rand.Source) bool {
	chance := hb.InvalidChance[ep]
	if chance <= 0 {
		return false
	}
	return (intN(s, chance) == 0)
}

// MapConfig provides configuration for a stress/load test.
type MapConfig struct {
	MapID         int64
	MetricFactory monitoring.MetricFactory
	Client        trillian.TrillianMapClient
	Admin         trillian.TrillianAdminClient
	RandSource    rand.Source
	EPBias        MapBias
	Operations    uint64
	EmitInterval  time.Duration
	IgnoreErrors  bool
}

// String conforms with Stringer for MapConfig.
func (c MapConfig) String() string {
	return fmt.Sprintf("mapID:%d biases:{%v} #operations:%d emit every:%v ignoreErrors? %t",
		c.MapID, c.EPBias, c.Operations, c.EmitInterval, c.IgnoreErrors)
}

// HitMap performs load/stress operations according to given config.
func HitMap(cfg MapConfig) error {
	ctx := context.Background()

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

	for count := uint64(1); count < cfg.Operations; count++ {
		if err := s.retryOneOp(ctx); err != nil {
			return err
		}
	}
	glog.Infof("%d: completed %d operations on map", cfg.MapID, cfg.Operations)
	ticker.Stop()

	return nil
}

// hammerState tracks the operations that have been performed during a test run.
type hammerState struct {
	cfg      *MapConfig
	verifier *client.MapVerifier

	// prng is not thread-safe and should only be used from the main hammer
	// goroutine for reproducability.
	prng rand.Source

	// copies of earlier contents of the map
	prevContents versionedMapContents

	mu sync.RWMutex // Protects everything below

	// SMRs are arranged from later to earlier (so [0] is the most recent), and the
	// discovery of new SMRs will push older ones off the end.
	smr [smrCount]*trillian.SignedMapRoot

	// Counters for generating unique keys/values.
	keyIdx   int
	valueIdx int
}

func intN(prng rand.Source, n int) int {
	x := prng.Int63()
	// This is not uniformly distributed on [0, n), but that's not worth
	// bothering to adjust here.
	return int(x % int64(n))
}

func newHammerState(ctx context.Context, cfg *MapConfig) (*hammerState, error) {
	tree, err := cfg.Admin.GetTree(ctx, &trillian.GetTreeRequest{TreeId: cfg.MapID})
	if err != nil {
		return nil, fmt.Errorf("failed to get tree information: %v", err)
	}
	glog.Infof("%d: hammering tree with configuration %+v", cfg.MapID, tree)
	verifier, err := client.NewMapVerifierFromTree(tree)
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
	return &hammerState{
		cfg:      cfg,
		prng:     cfg.RandSource,
		verifier: verifier,
	}, nil
}

func (s *hammerState) nextKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keyIdx++
	return fmt.Sprintf("key-%08d", s.keyIdx)
}

func (s *hammerState) nextValue() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.valueIdx++
	return []byte(fmt.Sprintf("value-%09d", s.valueIdx))
}

func (s *hammerState) label() string {
	return strconv.FormatInt(s.cfg.MapID, 10)
}

func (s *hammerState) String() string {
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
	smr := s.previousSMR(0)
	return fmt.Sprintf("%d: lastSMR.rev=%s ops: total=%d invalid=%d errs=%v%s", s.cfg.MapID, smrRev(smr), totalReqs, totalInvalidReqs, totalErrs, details)
}

func (s *hammerState) pushSMR(smr *trillian.SignedMapRoot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Shuffle earlier SMRs along.
	for i := smrCount - 1; i > 0; i-- {
		s.smr[i] = s.smr[i-1]
	}

	s.smr[0] = smr
}

func (s *hammerState) previousSMR(which int) *trillian.SignedMapRoot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.smr[which]
}

func (s *hammerState) chooseOp() MapEntrypointName {
	return s.cfg.EPBias.choose(s.prng)
}

func (s *hammerState) chooseInvalid(ep MapEntrypointName) bool {
	return s.cfg.EPBias.invalid(ep, s.prng)
}

func (s *hammerState) retryOneOp(ctx context.Context) (err error) {
	ep := s.chooseOp()
	if s.chooseInvalid(ep) {
		glog.V(3).Infof("%d: perform invalid %s operation", s.cfg.MapID, ep)
		invalidReqs.Inc(s.label(), string(ep))
		return s.performInvalidOp(ctx, ep)
	}

	glog.V(3).Infof("%d: perform %s operation", s.cfg.MapID, ep)
	defer func(start time.Time) {
		rspLatency.Observe(time.Since(start).Seconds(), s.label(), string(ep))
	}(time.Now())

	deadline := time.Now().Add(maxRetryDuration)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	done := false
	for !done {
		reqs.Inc(s.label(), string(ep))
		err = s.performOp(ctx, ep)

		switch err.(type) {
		case nil:
			rsps.Inc(s.label(), string(ep))
			done = true
		case errSkip:
			err = nil
			done = true
		case errInvariant:
			// Ensure invariant failures are not ignorable.  They indicate a design assumption
			// being broken or incorrect, so must be seen.
			done = true
		default:
			errs.Inc(s.label(), string(ep))
			if s.cfg.IgnoreErrors {
				glog.Warningf("%d: op %v failed (will retry): %v", s.cfg.MapID, ep, err)
			} else {
				done = true
			}
		}

		if time.Now().After(deadline) {
			glog.Warningf("%d: gave up retrying failed op %v after %v, returning last err %v", s.cfg.MapID, ep, maxRetryDuration, err)
			done = true
		}
	}
	return err
}

func (s *hammerState) performOp(ctx context.Context, ep MapEntrypointName) error {
	switch ep {
	case GetLeavesName:
		return s.getLeaves(ctx)
	case GetLeavesRevName:
		return s.getLeavesRev(ctx)
	case SetLeavesName:
		return s.setLeaves(ctx)
	case GetSMRName:
		return s.getSMR(ctx)
	case GetSMRRevName:
		return s.getSMRRev(ctx)
	default:
		return fmt.Errorf("internal error: unknown entrypoint %s selected for valid request", ep)
	}
}

func (s *hammerState) performInvalidOp(ctx context.Context, ep MapEntrypointName) error {
	switch ep {
	case GetLeavesName:
		return s.getLeavesInvalid(ctx)
	case GetLeavesRevName:
		return s.getLeavesRevInvalid(ctx)
	case SetLeavesName:
		return s.setLeavesInvalid(ctx)
	case GetSMRRevName:
		return s.getSMRRevInvalid(ctx)
	case GetSMRName:
		return fmt.Errorf("no invalid request possible for entrypoint %s", ep)
	default:
		return fmt.Errorf("internal error: unknown entrypoint %s selected for invalid request", ep)
	}
}

func (s *hammerState) getLeaves(ctx context.Context) error {
	return s.doGetLeaves(ctx, true /*latest*/)
}

func (s *hammerState) getLeavesRev(ctx context.Context) error {
	return s.doGetLeaves(ctx, false /*latest*/)
}

func (s *hammerState) doGetLeaves(ctx context.Context, latest bool) error {
	choices := []Choice{ExistingKey, NonexistentKey}

	if s.prevContents.empty() {
		glog.V(3).Infof("%d: skipping get-leaves as no data yet", s.cfg.MapID)
		return errSkip{}
	}
	var contents *mapContents
	if latest {
		contents = s.prevContents.lastCopy()
	} else {
		contents = s.prevContents.pickCopy(s.prng)
	}

	n := intN(s.prng, 10) // can be zero
	indices := make([][]byte, n)
	for i := 0; i < n; i++ {
		choice := choices[intN(s.prng, len(choices))]
		if contents.empty() {
			choice = NonexistentKey
		}
		switch choice {
		case ExistingKey:
			// No duplicate removal, so we can end up asking for same key twice in the same request.
			key := contents.pickKey(s.prng)
			indices[i] = key
		case NonexistentKey:
			indices[i] = testonly.TransparentHash("non-existent-key")
		}
	}

	var rsp *trillian.GetMapLeavesResponse
	var rqMsg proto.Message
	label := "get-leaves"
	var err error
	if latest {
		req := &trillian.GetMapLeavesRequest{
			MapId: s.cfg.MapID,
			Index: indices,
		}
		rsp, err = s.cfg.Client.GetLeaves(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to %s(%+v): %v", label, req, err)
		}
		rqMsg = req
	} else {
		label += "-rev"
		req := &trillian.GetMapLeavesByRevisionRequest{
			MapId:    s.cfg.MapID,
			Revision: int64(contents.rev),
			Index:    indices,
		}
		rsp, err = s.cfg.Client.GetLeavesByRevision(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to %s(%+v): %v", label, req, err)
		}
		rqMsg = req
	}

	if glog.V(3) {
		dumpRespKeyVals(rsp.MapLeafInclusion)
	}

	root, err := s.verifier.VerifySignedMapRoot(rsp.MapRoot)
	if err != nil {
		return err
	}
	for _, inc := range rsp.MapLeafInclusion {
		if err := s.verifier.VerifyMapLeafInclusionHash(root.RootHash, inc); err != nil {
			return err
		}
	}

	if err := contents.checkContents(rsp.MapLeafInclusion); err != nil {
		return fmt.Errorf("incorrect contents of %s(%+v): %v", label, rqMsg, err)
	}
	glog.V(2).Infof("%d: got %d leaves, with SMR(time=%q, rev=%d)", s.cfg.MapID, len(rsp.MapLeafInclusion), time.Unix(0, int64(root.TimestampNanos)), root.Revision)
	return nil
}

func dumpRespKeyVals(incls []*trillian.MapLeafInclusion) {
	fmt.Println("Rsp key-vals:")
	for _, inc := range incls {
		var key mapKey
		copy(key[:], inc.Leaf.Index)
		leafVal := inc.Leaf.LeafValue
		fmt.Printf("k: %v -> v: %v\n", string(key[:]), string(leafVal))
	}
	fmt.Println("~~~~~~~~~~~~~")
}

func (s *hammerState) getLeavesInvalid(ctx context.Context) error {
	key := testonly.TransparentHash("..invalid-size")
	req := trillian.GetMapLeavesRequest{
		MapId: s.cfg.MapID,
		Index: [][]byte{key[2:]},
	}
	rsp, err := s.cfg.Client.GetLeaves(ctx, &req)
	if err == nil {
		return fmt.Errorf("unexpected success: get-leaves(MalformedKey: %+v): %+v", req, rsp.MapRoot)
	}
	glog.V(2).Infof("%d: expected failure: get-leaves(MalformedKey: %+v): %+v", s.cfg.MapID, req, rsp)
	return nil
}

func (s *hammerState) getLeavesRevInvalid(ctx context.Context) error {
	choices := []Choice{MalformedKey, RevTooBig, RevIsNegative}

	req := trillian.GetMapLeavesByRevisionRequest{MapId: s.cfg.MapID}
	contents := s.prevContents.lastCopy()
	choice := choices[intN(s.prng, len(choices))]

	rev := int64(0)
	var index []byte
	if contents == nil {
		// No contents so we can't choose a key
		choice = MalformedKey
	} else {
		rev = contents.rev
		index = contents.pickKey(s.prng)
	}
	switch choice {
	case MalformedKey:
		key := testonly.TransparentHash("..invalid-size")
		req.Index = [][]byte{key[2:]}
		req.Revision = rev
	case RevTooBig:
		req.Index = [][]byte{index}
		req.Revision = rev + invalidStretch
	case RevIsNegative:
		req.Index = [][]byte{index}
		req.Revision = -rev - invalidStretch
	}
	rsp, err := s.cfg.Client.GetLeavesByRevision(ctx, &req)
	if err == nil {
		return fmt.Errorf("unexpected success: get-leaves-rev(%v: %+v): %+v", choice, req, rsp.MapRoot)
	}
	glog.V(2).Infof("%d: expected failure: get-leaves-rev(%v: %+v): %+v", s.cfg.MapID, choice, req, rsp)
	return nil
}

func (s *hammerState) setLeaves(ctx context.Context) error {
	choices := []Choice{CreateLeaf, UpdateLeaf, DeleteLeaf}

	n := 1 + intN(s.prng, 10)
	leaves := make([]*trillian.MapLeaf, 0, n)
	contents := s.prevContents.lastCopy()
	rev := int64(0)
	if contents != nil {
		rev = contents.rev
	}
leafloop:
	for i := 0; i < n; i++ {
		choice := choices[intN(s.prng, len(choices))]
		if contents == nil || contents.empty() {
			choice = CreateLeaf
		}
		switch choice {
		case CreateLeaf:
			key := s.nextKey()
			value := s.nextValue()
			leaves = append(leaves, &trillian.MapLeaf{
				Index:     testonly.TransparentHash(key),
				LeafValue: value,
				ExtraData: []byte("extra-" + string(value)),
			})
			glog.V(3).Infof("%d: %v: data[%q]=%q", s.cfg.MapID, choice, key, string(value))
		case UpdateLeaf, DeleteLeaf:
			key := contents.pickKey(s.prng)
			// Not allowed to have the same key more than once in the same request
			for _, leaf := range leaves {
				if bytes.Equal(leaf.Index, key) {
					break leafloop
				}
			}
			var value, extra []byte
			if choice == UpdateLeaf {
				value = s.nextValue()
				extra = []byte("extra-" + string(value))
			}
			leaves = append(leaves, &trillian.MapLeaf{Index: key, LeafValue: value, ExtraData: extra})
			glog.V(3).Infof("%d: %v: data[%q]=%q (extra=%q)", s.cfg.MapID, choice, dehash(key), string(value), string(extra))
		}
	}
	req := trillian.SetMapLeavesRequest{
		MapId:    s.cfg.MapID,
		Leaves:   leaves,
		Metadata: metadataForRev(uint64(rev + 1)),
	}
	rsp, err := s.cfg.Client.SetLeaves(ctx, &req)
	if err != nil {
		return fmt.Errorf("failed to set-leaves(%+v): %v", req, err)
	}
	root, err := s.verifier.VerifySignedMapRoot(rsp.MapRoot)
	if err != nil {
		return err
	}

	s.pushSMR(rsp.MapRoot)
	if err := s.prevContents.updateContentsWith(root.Revision, leaves); err != nil {
		return err
	}
	glog.V(2).Infof("%d: set %d leaves, new SMR(time=%q, rev=%d)", s.cfg.MapID, len(leaves), time.Unix(0, int64(root.TimestampNanos)), root.Revision)
	return nil
}

func (s *hammerState) setLeavesInvalid(ctx context.Context) error {
	choices := []Choice{MalformedKey, DuplicateKey}

	var leaves []*trillian.MapLeaf
	value := []byte("value-for-invalid-req")

	choice := choices[intN(s.prng, len(choices))]
	contents := s.prevContents.lastCopy()
	if contents == nil || contents.empty() {
		choice = MalformedKey
	}
	switch choice {
	case MalformedKey:
		key := testonly.TransparentHash("..invalid-size")
		leaves = append(leaves, &trillian.MapLeaf{Index: key[2:], LeafValue: value})
	case DuplicateKey:
		key := contents.pickKey(s.prng)
		leaves = append(leaves, &trillian.MapLeaf{Index: key, LeafValue: value})
		leaves = append(leaves, &trillian.MapLeaf{Index: key, LeafValue: value})
	}
	req := trillian.SetMapLeavesRequest{MapId: s.cfg.MapID, Leaves: leaves}
	rsp, err := s.cfg.Client.SetLeaves(ctx, &req)
	if err == nil {
		return fmt.Errorf("unexpected success: set-leaves(%v: %+v): %+v", choice, req, rsp.MapRoot)
	}
	glog.V(2).Infof("%d: expected failure: set-leaves(%v: %+v): %+v", s.cfg.MapID, choice, req, rsp)
	return nil
}

func (s *hammerState) getSMR(ctx context.Context) error {
	req := trillian.GetSignedMapRootRequest{MapId: s.cfg.MapID}
	rsp, err := s.cfg.Client.GetSignedMapRoot(ctx, &req)
	if err != nil {
		return fmt.Errorf("failed to get-smr: %v", err)
	}
	root, err := s.verifier.VerifySignedMapRoot(rsp.MapRoot)
	if err != nil {
		return err
	}
	if got, want := string(root.Metadata), string(metadataForRev(root.Revision)); got != want {
		return fmt.Errorf("map metadata=%q; want %q", got, want)
	}

	s.pushSMR(rsp.MapRoot)
	glog.V(2).Infof("%d: got SMR(time=%q, rev=%d)", s.cfg.MapID, time.Unix(0, int64(root.TimestampNanos)), root.Revision)
	return nil
}

func (s *hammerState) getSMRRev(ctx context.Context) error {
	which := intN(s.prng, smrCount)
	smr := s.previousSMR(which)
	if smr == nil || len(smr.MapRoot) == 0 {
		glog.V(3).Infof("%d: skipping get-smr-rev as no earlier SMR", s.cfg.MapID)
		return errSkip{}
	}
	smrRoot, err := s.verifier.VerifySignedMapRoot(smr)
	if err != nil {
		return err
	}
	rev := int64(smrRoot.Revision)

	rsp, err := s.cfg.Client.GetSignedMapRootByRevision(ctx,
		&trillian.GetSignedMapRootByRevisionRequest{MapId: s.cfg.MapID, Revision: rev})
	if err != nil {
		return fmt.Errorf("failed to get-smr-rev(@%d): %v", rev, err)
	}
	root, err := s.verifier.VerifySignedMapRoot(rsp.MapRoot)
	if err != nil {
		return err
	}
	glog.V(2).Infof("%d: got SMR(time=%q, rev=%d)", s.cfg.MapID, time.Unix(0, int64(root.TimestampNanos)), root.Revision)

	if !proto.Equal(rsp.MapRoot, smr) {
		return fmt.Errorf("get-smr-rev(@%d)=%+v, want %+v", rev, rsp.MapRoot, smr)
	}
	return nil
}

func (s *hammerState) getSMRRevInvalid(ctx context.Context) error {
	choices := []Choice{RevTooBig, RevIsNegative}

	rev := latestRevision
	contents := s.prevContents.lastCopy()
	if contents != nil {
		rev = contents.rev
	}

	choice := choices[intN(s.prng, len(choices))]

	switch choice {
	case RevTooBig:
		rev += invalidStretch
	case RevIsNegative:
		rev = -invalidStretch
	}
	req := trillian.GetSignedMapRootByRevisionRequest{MapId: s.cfg.MapID, Revision: rev}
	rsp, err := s.cfg.Client.GetSignedMapRootByRevision(ctx, &req)
	if err == nil {
		return fmt.Errorf("unexpected success: get-smr-rev(%v: @%d): %+v", choice, rev, rsp.MapRoot)
	}
	glog.V(2).Infof("%d: expected failure: get-smr-rev(%v: @%d): %+v", s.cfg.MapID, choice, rev, rsp)
	return nil
}

func smrRev(smr *trillian.SignedMapRoot) string {
	if smr == nil {
		return "n/a"
	}
	var root types.MapRootV1
	if err := root.UnmarshalBinary(smr.MapRoot); err != nil {
		return "unknown"
	}
	return fmt.Sprintf("%d", root.Revision)
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
