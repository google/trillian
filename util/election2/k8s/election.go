package k8s

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/trillian/util/election2"
	v1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	coordinationv1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
)

// Factory creates Election instances.
type Factory struct {
	client        coordinationv1.CoordinationV1Interface
	instanceID    string
	namespace     string
	leaseDuration time.Duration
	retryPeriod   time.Duration
}

// NewElection creates a specific Election instance.
func (f *Factory) NewElection(ctx context.Context, resourceID string) (election2.Election, error) {
	el := Election{
		lock: &resourcelock.LeaseLock{
			LeaseMeta: metav1.ObjectMeta{
				Name:      strings.ToLower(resourceID),
				Namespace: f.namespace,
			},
			Client: f.client,
			LockConfig: resourcelock.ResourceLockConfig{
				Identity: f.instanceID,
			},
		},
		clock:         clock.RealClock{},
		leaseDuration: f.leaseDuration,
		retryPeriod:   f.retryPeriod,
		client:        f.client,
	}
	el.onLeaseChanged = sync.NewCond(&el.mu)
	err := el.watchLease(ctx, el.onLeaseChanged)
	if err != nil {
		return nil, err
	}

	return &el, nil
}

type Election struct {
	lock *resourcelock.LeaseLock

	client coordinationv1.CoordinationV1Interface

	// clock is wrapper around time to allow for less flaky testing
	clock clock.Clock

	// internal bookkeeping
	observedRecord    resourcelock.LeaderElectionRecord
	observedRawRecord []byte
	observedTime      time.Time
	// used to lock the observedRecord
	observedRecordLock sync.Mutex

	leaseDuration time.Duration
	retryPeriod   time.Duration

	mu             sync.Mutex
	onLeaseChanged *sync.Cond
}

func (e *Election) Await(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	err := wait.PollUntilContextCancel(ctx, e.retryPeriod, true, func(ctx context.Context) (done bool, err error) {
		succeeded := e.tryAcquireOrRenew(ctx)
		if !succeeded {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (e *Election) tryAcquireOrRenew(ctx context.Context) bool {
	now := metav1.NewTime(e.clock.Now())
	leaderElectionRecord := resourcelock.LeaderElectionRecord{
		HolderIdentity:       e.lock.Identity(),
		LeaseDurationSeconds: int(e.leaseDuration / time.Second),
		RenewTime:            now,
		AcquireTime:          now,
	}

	// 1. fast path for the leader to update optimistically assuming that the record observed
	// last time is the current version.
	if e.IsLeader() && e.isLeaseValid(now.Time) {
		oldObservedRecord := e.getObservedRecord()
		leaderElectionRecord.AcquireTime = oldObservedRecord.AcquireTime
		leaderElectionRecord.LeaderTransitions = oldObservedRecord.LeaderTransitions

		err := e.lock.Update(ctx, leaderElectionRecord)
		if err == nil {
			e.setObservedRecord(&leaderElectionRecord)
			return true
		}
		klog.Errorf("Failed to update lock optimistically: %v, falling back to slow path", err)
	}

	// 2. obtain or create the ElectionRecord
	oldLeaderElectionRecord, oldLeaderElectionRawRecord, err := e.lock.Get(ctx)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			klog.Errorf("error retrieving resource lock %v: %v", e.lock.Describe(), err)
			return false
		}
		if err = e.lock.Create(ctx, leaderElectionRecord); err != nil {
			klog.Errorf("error initially creating leader election record: %v", err)
			return false
		}

		e.setObservedRecord(&leaderElectionRecord)

		return true
	}

	// 3. Record obtained, check the Identity & Time
	if !bytes.Equal(e.observedRawRecord, oldLeaderElectionRawRecord) {
		e.setObservedRecord(oldLeaderElectionRecord)

		e.observedRawRecord = oldLeaderElectionRawRecord
	}
	if len(oldLeaderElectionRecord.HolderIdentity) > 0 && e.isLeaseValid(now.Time) && !e.IsLeader() {
		klog.V(4).Infof("lock is held by %v and has not yet expired", oldLeaderElectionRecord.HolderIdentity)
		return false
	}

	// 4. We're going to try to update. The leaderElectionRecord is set to it's default
	// here. Let's correct it before updating.
	if e.IsLeader() {
		leaderElectionRecord.AcquireTime = oldLeaderElectionRecord.AcquireTime
		leaderElectionRecord.LeaderTransitions = oldLeaderElectionRecord.LeaderTransitions
	} else {
		leaderElectionRecord.LeaderTransitions = oldLeaderElectionRecord.LeaderTransitions + 1
	}

	// update the lock itself
	if err = e.lock.Update(ctx, leaderElectionRecord); err != nil {
		klog.Errorf("Failed to update lock: %v", err)
		return false
	}

	e.setObservedRecord(&leaderElectionRecord)
	return true
}

// WithMastership returns a context that is canceled if mastership is lost.
func (e *Election) WithMastership(ctx context.Context) (context.Context, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.IsLeader() {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		return cctx, nil
	}

	cctx, cancel := e.watchContext(ctx, &e.mu, e.onLeaseChanged)

	go func() { // Goroutine to cancel the mastership context when leadership is lost.
		defer cancel()
		e.mu.Lock()
		defer e.mu.Unlock()

		now := metav1.NewTime(e.clock.Now())
		for e.IsLeader() && e.isLeaseValid(now.Time) && cctx.Err() == nil {
			e.onLeaseChanged.Wait()
		}
	}()

	return cctx, nil
}

func (e *Election) watchContext(ctx context.Context, l sync.Locker, cond *sync.Cond) (context.Context, context.CancelFunc) {
	cctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		<-cctx.Done()
		l.Lock() // Avoid racing with cond waiters on ctx status.
		defer l.Unlock()
		cond.Broadcast()
	}()
	return cctx, cancel
}

func (e *Election) watchLease(ctx context.Context, onLeaseChanged *sync.Cond) error {
	watcher, err := e.client.Leases(e.lock.LeaseMeta.Namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", e.lock.LeaseMeta.Name),
	})
	if err != nil {
		return fmt.Errorf("lease watcher `%s` failed: %w", e.lock.LockConfig.Identity, err)
	}
	go func() {
		defer watcher.Stop()
		defer onLeaseChanged.Broadcast()

		channel := watcher.ResultChan()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-channel:
				if !ok {
					// channel closed
					return
				}
				switch event.Type {
				case watch.Modified, watch.Added:
					lease, ok := event.Object.(*v1.Lease)
					if !ok {
						continue
					}
					record := resourcelock.LeaseSpecToLeaderElectionRecord(&lease.Spec)
					e.setObservedRecord(record)
				case watch.Deleted:
					e.setObservedRecord(&resourcelock.LeaderElectionRecord{})
				}
			}
		}
	}()
	return nil
}

func (e *Election) Resign(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	defer e.onLeaseChanged.Broadcast()

	if !e.IsLeader() {
		return nil
	}

	now := metav1.NewTime(e.clock.Now())
	leaderElectionRecord := resourcelock.LeaderElectionRecord{
		LeaderTransitions:    e.getObservedRecord().LeaderTransitions,
		LeaseDurationSeconds: 1,
		RenewTime:            now,
		AcquireTime:          now,
	}

	if err := e.lock.Update(ctx, leaderElectionRecord); err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	e.setObservedRecord(&leaderElectionRecord)
	return nil
}

func (e *Election) Close(ctx context.Context) error {
	return e.Resign(ctx)
}

func (e *Election) isLeaseValid(now time.Time) bool {
	duration := e.getObservedRecord().LeaseDurationSeconds

	e.observedRecordLock.Lock()
	defer e.observedRecordLock.Unlock()

	return e.observedTime.Add(time.Second * time.Duration(duration)).After(now)
}

// setObservedRecord will set a new observedRecord and update observedTime to the current time.
// Protect critical sections with lock.
func (e *Election) setObservedRecord(observedRecord *resourcelock.LeaderElectionRecord) {
	e.observedRecordLock.Lock()
	defer e.observedRecordLock.Unlock()

	e.observedRecord = *observedRecord
	e.observedTime = e.clock.Now()

	e.onLeaseChanged.Broadcast()
}

// getObservedRecord returns observersRecord.
// Protect critical sections with lock.
func (e *Election) getObservedRecord() resourcelock.LeaderElectionRecord {
	e.observedRecordLock.Lock()
	defer e.observedRecordLock.Unlock()

	return e.observedRecord
}

// GetLeader returns the identity of the last observed leader or returns the empty string if
// no leader has yet been observed.
// This function is for informational purposes. (e.g. monitoring, logs, etc.)
func (e *Election) GetLeader() string {
	return e.getObservedRecord().HolderIdentity
}

// IsLeader returns true if the last observed leader was this client else returns false.
func (e *Election) IsLeader() bool {
	return e.getObservedRecord().HolderIdentity == e.lock.Identity()
}
