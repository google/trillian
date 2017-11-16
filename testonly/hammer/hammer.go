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
	"crypto/sha256"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/testonly"
)

const (
	defaultEmitSeconds = 10
	// How many SMRs to hold on to.
	smrCount = 30
	// How many copies of map contents to hold on to.
	copyCount  = 10
	latestCopy = 0
	// How far beyond current revision to request for invalid requests
	invalidStretch = int64(10000)
	// rev=-1 is used when requesting the latest revision
	latestRevision = int64(-1)
)

var (
	maxRetryDuration = 60 * time.Second
)

var (
	// Metrics are all per-map (label "mapid"), but may also be
	// per-entrypoint (label "ep") or per-return-code (label "rc").
	once        sync.Once
	reqs        monitoring.Counter // mapid, ep => value
	errs        monitoring.Counter // mapid, ep => value
	rsps        monitoring.Counter // mapid, ep, rc => value
	invalidReqs monitoring.Counter // mapid, ep => value
)

// setupMetrics initializes all the exported metrics.
func setupMetrics(mf monitoring.MetricFactory) {
	reqs = mf.NewCounter("reqs", "Number of valid requests sent", "mapid", "ep")
	errs = mf.NewCounter("errs", "Number of error responses received for valid requests", "mapid", "ep")
	rsps = mf.NewCounter("rsps", "Number of responses received for valid requests", "mapid", "ep")
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
	GetLeavesName = MapEntrypointName("GetLeaves")
	SetLeavesName = MapEntrypointName("SetLeaves")
	GetSMRName    = MapEntrypointName("GetSMR")
	GetSMRRevName = MapEntrypointName("GetSMRRev")
)

var mapEntrypoints = []MapEntrypointName{GetLeavesName, SetLeavesName, GetSMRName, GetSMRRevName}

// Choice is a readable representation of a choice about how to perform a hammering operation.
type Choice string

// Constants for both valid and invalid operation choices.
const (
	ExistingKey    = Choice("ExistingKey")
	NonexistentKey = Choice("NonexistentKey")
	MalformedKey   = Choice("MalformedKey")
	DuplicateKey   = Choice("DuplicateKey")
	RevTooBig      = Choice("RevTooBig")
	RevIsZero      = Choice("RevIsZero")
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

// Choose randomly picks an operation to perform according to the biases.
func (hb *MapBias) Choose() MapEntrypointName {
	if hb.total == 0 {
		for _, ep := range mapEntrypoints {
			hb.total += hb.Bias[ep]
		}
	}
	which := rand.Intn(hb.total)
	for _, ep := range mapEntrypoints {
		which -= hb.Bias[ep]
		if which < 0 {
			return ep
		}
	}
	panic("random choice out of range")
}

// Invalid randomly chooses whether an operation should be invalid.
func (hb *MapBias) Invalid(ep MapEntrypointName) bool {
	chance := hb.InvalidChance[ep]
	if chance <= 0 {
		return false
	}
	return (rand.Intn(chance) == 0)
}

// MapConfig provides configuration for a stress/load test.
type MapConfig struct {
	MapID         int64
	MetricFactory monitoring.MetricFactory
	Client        trillian.TrillianMapClient
	EPBias        MapBias
	Operations    uint64
	EmitInterval  time.Duration
	IgnoreErrors  bool
	// TODO(phad): remove CheckSignatures when downstream dependencies no longer need it,
	// i.e. when all Map storage implementations support signature storage.
	CheckSignatures bool
}

// String conforms with Stringer for MapConfig.
func (c MapConfig) String() string {
	return fmt.Sprintf("mapID:%d biases:{%v} #operations:%d emit every:%v ignoreErrors? %t checkSignatures? %t",
		c.MapID, c.EPBias, c.Operations, c.EmitInterval, c.IgnoreErrors, c.CheckSignatures)
}

// HitMap performs load/stress operations according to given config.
func HitMap(cfg MapConfig) error {
	ctx := context.Background()

	s := newHammerState(&cfg)

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

type mapKey [sha256.Size]byte
type mapContents struct {
	data map[mapKey]string
	rev  int64
}

// hammerState tracks the operations that have been performed during a test run.
type hammerState struct {
	cfg *MapConfig

	mu sync.RWMutex // Protects everything below

	// SMRs are arranged from later to earlier (so [0] is the most recent), and the
	// discovery of new SMRs will push older ones off the end.
	smr [smrCount]*trillian.SignedMapRoot

	// contents holds copies of (our contents in) the map at different revisions,
	// from later to earlier (so [0] is the most recent).
	contents [copyCount]mapContents

	// Counters for generating unique keys/values.
	keyIdx   int
	valueIdx int
}

func newHammerState(cfg *MapConfig) *hammerState {
	mf := cfg.MetricFactory
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}
	once.Do(func() { setupMetrics(mf) })
	if cfg.EmitInterval == 0 {
		cfg.EmitInterval = defaultEmitSeconds * time.Second
	}
	return &hammerState{cfg: cfg}
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

// rev returns the revision of the specified copy of the contents, or panics if that copy doesn't exist.
func (s *hammerState) rev(which int) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.contents[which].data == nil {
		panic(fmt.Sprintf("rev(): contents[%d] has no stored data", which))
	}
	return s.contents[which].rev
}

func (s *hammerState) empty(which int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.contents[which].data) == 0
}

func (s *hammerState) label() string {
	return strconv.FormatInt(s.cfg.MapID, 10)
}

func (s *hammerState) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	return fmt.Sprintf("%d: lastSMR.rev=%s ops: total=%d invalid=%d errs=%v%s", s.cfg.MapID, smrRev(s.smr[0]), totalReqs, totalInvalidReqs, totalErrs, details)
}

func (s *hammerState) pushSMR(smr *trillian.SignedMapRoot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Shuffle earlier SMRs along.
	for i := smrCount - 1; i > 0; i-- {
		s.smr[i] = s.smr[i-1]
	}

	if !s.cfg.CheckSignatures {
		smr.Signature = nil
	}
	s.smr[0] = smr
}

func (s *hammerState) previousSMR(which int) *trillian.SignedMapRoot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r := s.smr[which]
	if !s.cfg.CheckSignatures && r != nil && r.Signature != nil {
		panic(fmt.Sprintf("signature should have been cleared before storing SMR %v", r))
	}
	return r
}

// pickKey randomly select a key that already exists in a particular
// copy of the map's contents. Assumes that the selected copy is non-empty.
func (s *hammerState) pickKey(which int) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	l := len(s.contents[which].data)
	if l == 0 {
		panic(fmt.Sprintf("internal error: can't pick a key, map contents[%d] is empty!", which))
	}

	choice := rand.Intn(l)
	for k := range s.contents[which].data {
		if choice == 0 {
			return k[:]
		}
		choice--
	}
	panic("internal error: inconsistent count of map contents")
}

// pickCopy returns an index for a non-empty copy of the map's
// contents.  If there are no non-empty copies, it returns -1, false.
func (s *hammerState) pickCopy() (int, bool) {
	// Count the number of filled copies.
	i := 0
	for ; i < copyCount; i++ {
		if len(s.contents[i].data) == 0 {
			break
		}
	}
	if i == 0 {
		// No copied contents yet
		return -1, false
	}
	return rand.Intn(i), true
}

func (s *hammerState) updateContents(rev int64, leaves []*trillian.MapLeaf) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Sanity check on rev being +ve.
	if rev < 1 {
		return errInvariant{fmt.Sprintf("got rev %d, want >=1 when trying to update hammer state with contents", rev)}
	}

	// Shuffle earlier contents along.
	for i := copyCount - 1; i > 0; i-- {
		s.contents[i] = s.contents[i-1]
	}
	// Start from previous map contents
	s.contents[0].rev = rev
	s.contents[0].data = make(map[mapKey]string)
	for k, v := range s.contents[1].data {
		s.contents[0].data[k] = v
	}
	// Update with given data
	for _, leaf := range leaves {
		var k mapKey
		copy(k[:], leaf.Index)
		if leaf.LeafValue != nil {
			s.contents[0].data[k] = string(leaf.LeafValue)
		} else {
			delete(s.contents[0].data, k)
		}
	}

	// Sanity check on latest rev being > prev rev.
	if s.contents[0].rev <= s.contents[1].rev {
		return errInvariant{fmt.Sprintf("got rev %d, want >%d when trying to update hammer state with new contents", s.contents[0].rev, s.contents[1].rev)}
	}
	return nil
}

func (s *hammerState) checkContents(which int, leafInclusions []*trillian.MapLeafInclusion) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, inc := range leafInclusions {
		leaf := inc.Leaf
		var key mapKey
		copy(key[:], leaf.Index)
		value, ok := s.contents[which].data[key]
		if ok {
			if string(leaf.LeafValue) != value {
				return fmt.Errorf("got leaf[%v].LeafValue=%q, want %q", key, leaf.LeafValue, value)
			}
			if want := "extra-" + value; string(leaf.ExtraData) != want {
				return fmt.Errorf("got leaf[%v].ExtraData=%q, want %q", key, leaf.ExtraData, want)
			}
		} else {
			if len(leaf.LeafValue) > 0 {
				return fmt.Errorf("got leaf[%v].LeafValue=%q, want not-present", key, leaf.LeafValue)
			}
		}
	}
	return nil
}

func (s *hammerState) chooseOp() MapEntrypointName {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.EPBias.Choose()
}

func (s *hammerState) chooseInvalid(ep MapEntrypointName) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.EPBias.Invalid(ep)
}

func (s *hammerState) retryOneOp(ctx context.Context) (err error) {
	ep := s.chooseOp()
	if s.chooseInvalid(ep) {
		glog.V(3).Infof("%d: perform invalid %s operation", s.cfg.MapID, ep)
		invalidReqs.Inc(s.label(), string(ep))
		return s.performInvalidOp(ctx, ep)
	}

	glog.V(3).Infof("%d: perform %s operation", s.cfg.MapID, ep)

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
	choices := []Choice{ExistingKey, NonexistentKey}

	which, ok := s.pickCopy()
	if !ok {
		glog.V(3).Infof("%d: skipping get-leaves as no data yet", s.cfg.MapID)
		return errSkip{}
	}
	rev := s.rev(which)
	if which == 0 {
		// We're asking for the latest revision; this can also be requested
		// by using a negative revision. So do that sometimes.
		if rand.Intn(2) == 0 {
			rev = -1
		}
	}
	n := rand.Intn(10) // can be zero
	req := trillian.GetMapLeavesRequest{
		MapId:    s.cfg.MapID,
		Revision: rev,
		Index:    make([][]byte, n),
	}
	for i := 0; i < n; i++ {
		choice := choices[rand.Intn(len(choices))]
		switch choice {
		case ExistingKey:
			// No duplicate removal, so we can end up asking for same key twice in the same request.
			key := s.pickKey(which)
			req.Index[i] = key
		case NonexistentKey:
			req.Index[i] = testonly.TransparentHash("non-existent-key")
		}
	}
	rsp, err := s.cfg.Client.GetLeaves(ctx, &req)
	if err != nil {
		return fmt.Errorf("failed to get-leaves(%+v): %v", req, err)
	}

	// TODO(drysdale): verify inclusion
	if err := s.checkContents(which, rsp.MapLeafInclusion); err != nil {
		return fmt.Errorf("incorrect contents of get-leaves(%+v): %v", req, err)
	}
	glog.V(2).Infof("%d: got %d leaves, with SMR(time=%q, rev=%d)", s.cfg.MapID, len(rsp.MapLeafInclusion), timeFromNanos(rsp.MapRoot.TimestampNanos), rsp.MapRoot.MapRevision)
	return nil
}

func (s *hammerState) getLeavesInvalid(ctx context.Context) error {
	choices := []Choice{MalformedKey, RevTooBig}

	req := trillian.GetMapLeavesRequest{MapId: s.cfg.MapID}
	rev := latestRevision
	choice := choices[rand.Intn(len(choices))]
	if s.empty(latestCopy) {
		choice = MalformedKey
	} else {
		rev = s.rev(latestCopy)
	}
	switch choice {
	case MalformedKey:
		key := testonly.TransparentHash("..invalid-size")
		req.Index = [][]byte{key[2:]}
		req.Revision = rev
	case RevTooBig:
		req.Index = [][]byte{s.pickKey(latestCopy)}
		req.Revision = rev + invalidStretch
	}
	rsp, err := s.cfg.Client.GetLeaves(ctx, &req)
	if err == nil {
		return fmt.Errorf("unexpected success: get-leaves(%v: %+v): %+v", choice, req, rsp.MapRoot)
	}
	glog.V(2).Infof("%d: expected failure: get-leaves(%v: %+v): %+v", s.cfg.MapID, choice, req, rsp)
	return nil
}

func (s *hammerState) setLeaves(ctx context.Context) error {
	choices := []Choice{CreateLeaf, UpdateLeaf, DeleteLeaf}

	n := 1 + rand.Intn(10)
	leaves := make([]*trillian.MapLeaf, 0, n)
leafloop:
	for i := 0; i < n; i++ {
		choice := choices[rand.Intn(len(choices))]
		if s.empty(latestCopy) {
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
			key := s.pickKey(latestCopy)
			// Not allowed to have the same key more than once in the same request
			for _, leaf := range leaves {
				if bytes.Equal(leaf.Index, key) {
					break leafloop
				}
			}
			var value []byte
			if choice == UpdateLeaf {
				value = s.nextValue()
			}
			glog.V(3).Infof("%d: %v: data[%q]=%q", s.cfg.MapID, choice, dehash(key), string(value))
			extra := []byte("extra-" + string(value))
			leaves = append(leaves, &trillian.MapLeaf{Index: key, LeafValue: value, ExtraData: extra})
		}
	}
	req := trillian.SetMapLeavesRequest{
		MapId:  s.cfg.MapID,
		Leaves: leaves,
	}
	rsp, err := s.cfg.Client.SetLeaves(ctx, &req)
	if err != nil {
		return fmt.Errorf("failed to set-leaves(%+v): %v", req, err)
	}

	s.pushSMR(rsp.MapRoot)
	if err := s.updateContents(rsp.MapRoot.MapRevision, leaves); err != nil {
		return err
	}
	glog.V(2).Infof("%d: set %d leaves, new SMR(time=%q, rev=%d)", s.cfg.MapID, len(leaves), timeFromNanos(rsp.MapRoot.TimestampNanos), rsp.MapRoot.MapRevision)
	return nil
}

func (s *hammerState) setLeavesInvalid(ctx context.Context) error {
	choices := []Choice{MalformedKey, DuplicateKey}

	var leaves []*trillian.MapLeaf
	value := []byte("value-for-invalid-req")

	choice := choices[rand.Intn(len(choices))]

	if s.empty(latestCopy) {
		choice = MalformedKey
	}
	switch choice {
	case MalformedKey:
		key := testonly.TransparentHash("..invalid-size")
		leaves = append(leaves, &trillian.MapLeaf{Index: key[2:], LeafValue: value})
	case DuplicateKey:
		key := s.pickKey(latestCopy)
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

	// TODO(drysdale): check signature
	s.pushSMR(rsp.MapRoot)
	glog.V(2).Infof("%d: Got SMR(time=%q, rev=%d)", s.cfg.MapID, timeFromNanos(rsp.MapRoot.TimestampNanos), rsp.MapRoot.MapRevision)
	return nil
}

func (s *hammerState) getSMRRev(ctx context.Context) error {
	which := rand.Intn(smrCount)
	smr := s.previousSMR(which)
	if smr == nil || smr.MapRevision < 0 {
		glog.V(3).Infof("%d: skipping get-smr-rev as no earlier SMR", s.cfg.MapID)
		return errSkip{}
	}

	req := trillian.GetSignedMapRootByRevisionRequest{MapId: s.cfg.MapID, Revision: smr.MapRevision}
	rsp, err := s.cfg.Client.GetSignedMapRootByRevision(ctx, &req)
	if err != nil {
		return fmt.Errorf("failed to get-smr-rev(@%d): %v", req.Revision, err)
	}
	glog.V(2).Infof("%d: Got SMR(time=%q, rev=%d)", s.cfg.MapID, timeFromNanos(rsp.MapRoot.TimestampNanos), rsp.MapRoot.MapRevision)

	if !s.cfg.CheckSignatures {
		rsp.MapRoot.Signature = nil
	}

	if !proto.Equal(rsp.MapRoot, smr) {
		return fmt.Errorf("get-smr-rev(@%d)=%+v, want %+v", req.Revision, rsp.MapRoot, smr)
	}
	return nil
}

func (s *hammerState) getSMRRevInvalid(ctx context.Context) error {
	choices := []Choice{RevTooBig, RevIsNegative}

	rev := latestRevision
	if !s.empty(latestCopy) {
		rev = s.rev(latestCopy)
	}

	choice := choices[rand.Intn(len(choices))]

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

func timeFromNanos(nanos int64) time.Time {
	return time.Unix(nanos/1e9, nanos%1e9)
}

func smrRev(smr *trillian.SignedMapRoot) string {
	if smr == nil {
		return "n/a"
	}
	return fmt.Sprintf("%d", smr.MapRevision)
}

func dehash(index []byte) string {
	return strings.TrimRight(string(index), "\x00")
}
