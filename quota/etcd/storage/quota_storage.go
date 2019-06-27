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

// Package storage contains storage classes for etcd-based quotas.
package storage

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/etcd/storagepb"
	"github.com/google/trillian/util/clock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	configsKey = "quotas/configs"
)

var (
	timeSource = clock.System

	globalPattern *regexp.Regexp
	treesPattern  *regexp.Regexp
	usersPattern  *regexp.Regexp
)

func init() {
	var err error
	globalPattern, err = regexp.Compile("^quotas/global/(read|write)/config$")
	if err != nil {
		glog.Fatalf("bad global pattern: %v", err)
	}
	treesPattern, err = regexp.Compile(`^quotas/trees/\d+/(read|write)/config$`)
	if err != nil {
		glog.Fatalf("bad trees pattern: %v", err)
	}
	usersPattern, err = regexp.Compile("^quotas/users/[^/]+/(read|write)/config$")
	if err != nil {
		glog.Fatalf("bad users pattern: %v", err)
	}
}

// IsNameValid returns true if name is a valid quota name.
func IsNameValid(name string) bool {
	switch {
	case globalPattern.MatchString(name):
		return true
	case usersPattern.MatchString(name):
		return true
	case treesPattern.MatchString(name):
		// Tree ID must fit on an int64
		id := strings.Split(name, "/")[2]
		_, err := strconv.ParseInt(id, 10, 64)
		return err == nil
	}
	return false
}

// QuotaStorage is the interface between the etcd-based quota implementations (quota.Manager and
// RPCs) and etcd itself.
type QuotaStorage struct {
	Client *clientv3.Client
}

// UpdateConfigs creates or updates the supplied configs in etcd.
// If no config exists, the current config is assumed to be an empty storagepb.Configs proto.
// The update function allows for mask-based updates and ensures a single-transaction
// read-modify-write operation.
// If reset is true, all specified configs will be set to their max number of tokens. If false,
// existing quotas won't be modified, unless the max number of tokens is lowered, in which case
// the new ceiling is enforced.
// Newly created quotas are always set to max tokens, regardless of the reset parameter.
func (qs *QuotaStorage) UpdateConfigs(ctx context.Context, reset bool, update func(*storagepb.Configs)) (*storagepb.Configs, error) {
	if update == nil {
		return nil, status.Error(codes.Internal, "update function required")
	}

	var updated *storagepb.Configs
	_, err := concurrency.NewSTMSerializable(ctx, qs.Client, func(s concurrency.STM) error {
		previous, err := getConfigs(s)
		if err != nil {
			return err
		}
		// Take a deep copy of "previous". It's pointers all the way down, so it's easier to just
		// unmarshal it again. STM has the key we just read and it should be exactly the same as
		// previous...
		updated, err = getConfigs(s)
		if err != nil {
			return err
		}

		// ... but let's sanity check that the configs match, just in case.
		if !proto.Equal(previous, updated) {
			return status.Error(codes.Internal, "verification failed: previous quota config != updated")
		}

		update(updated)
		if err := validate(updated); err != nil {
			return err
		}

		if !proto.Equal(previous, updated) {
			pb, err := proto.Marshal(updated)
			if err != nil {
				return err
			}
			s.Put(configsKey, string(pb))
		}

		now := timeSource.Now()
		for _, cfg := range updated.Configs {
			// Make no distinction between enabled and disabled configs here. Get/Peek/Put are
			// prepared to handle it, and recording the bucket as if it were enabled allows us to
			// take advantage of the already-existing reset and lowering logic.
			key := bucketKey(cfg)

			var prev *storagepb.Config
			for _, p := range previous.Configs {
				if cfg.Name == p.Name {
					prev = p
					break
				}
			}

			switch {
			case prev == nil || prev.State == storagepb.Config_DISABLED || reset: // new bucket
				bucket := &storagepb.Bucket{
					Tokens:                        cfg.MaxTokens,
					LastReplenishMillisSinceEpoch: now.UnixNano() / 1e6,
				}
				pb, err := proto.Marshal(bucket)
				if err != nil {
					return err
				}
				s.Put(key, string(pb))
			case prev != nil && cfg.MaxTokens < prev.MaxTokens: // lowered bucket
				// modBucket will coerce tokens to cfg.MaxTokens, if necessary
				if _, err := modBucket(s, cfg, now, 0 /* add */); err != nil {
					return err
				}
			}
		}

		return nil
	})
	return updated, err
}

func validate(cfgs *storagepb.Configs) error {
	names := make(map[string]bool)
	for i, cfg := range cfgs.Configs {
		switch n := cfg.Name; {
		case n == "":
			return status.Errorf(codes.InvalidArgument, "config name is required (Configs[%v].Name is empty)", i)
		case !IsNameValid(cfg.Name):
			return status.Errorf(codes.InvalidArgument, "config name malformed (Configs[%v].Name = %q)", i, n)
		}
		if s := cfg.State; s == storagepb.Config_UNKNOWN_CONFIG_STATE {
			return status.Errorf(codes.InvalidArgument, "config state invalid (Configs[%v].State = %s)", i, s)
		}
		if t := cfg.MaxTokens; t <= 0 {
			return status.Errorf(codes.InvalidArgument, "config max tokens must be > 0 (Configs[%v].MaxTokens = %v)", i, t)
		}
		switch s := cfg.ReplenishmentStrategy.(type) {
		case *storagepb.Config_SequencingBased:
			if usersPattern.MatchString(cfg.Name) {
				return status.Errorf(codes.InvalidArgument, "user quotas cannot use sequencing-based replenishment (Configs[%v].ReplenishmentStrategy)", i)
			}
			if strings.HasSuffix(cfg.Name, "/read/config") {
				return status.Errorf(codes.InvalidArgument, "read quotas cannot use sequencing-based replenishment (Configs[%v].ReplenishmentStrategy)", i)
			}
		case *storagepb.Config_TimeBased:
			if t := s.TimeBased.TokensToReplenish; t <= 0 {
				return status.Errorf(codes.InvalidArgument, "time based tokens must be > 0 (Configs[%v].TimeBased.TokensToReplenish = %v)", i, t)
			}
			if r := s.TimeBased.ReplenishIntervalSeconds; r <= 0 {
				return status.Errorf(codes.InvalidArgument, "time based replenish interval must be > 0 (Configs[%v].TimeBased.ReplenishIntervalSeconds = %v)", i, r)
			}
		default:
			return status.Errorf(codes.InvalidArgument, "unsupported replenishment strategy (Configs[%v].ReplenishmentStrategy = %T)", i, s)
		}
		if names[cfg.Name] {
			return status.Errorf(codes.InvalidArgument, "duplicate config name found at Configs[%v].Name", i)
		}
		names[cfg.Name] = true
	}
	return nil
}

// Configs returns the currently known quota configs.
// If no config was explicitly created, then an empty storage.Configs proto is returned.
func (qs *QuotaStorage) Configs(ctx context.Context) (*storagepb.Configs, error) {
	var cfgs *storagepb.Configs
	_, err := concurrency.NewSTMSerializable(ctx, qs.Client, func(s concurrency.STM) error {
		var err error
		cfgs, err = getConfigs(s)
		return err

	})
	return cfgs, err
}

// Get acquires "tokens" tokens from the named quotas.
// If one of the specified quotas doesn't have enough tokens, the entire operation fails. Unknown or
// disabled quotas are considered infinite, therefore get requests will always succeed for them.
func (qs *QuotaStorage) Get(ctx context.Context, names []string, tokens int64) error {
	if tokens < 0 {
		return fmt.Errorf("invalid number of tokens: %v", tokens)
	}
	return qs.mod(ctx, names, -tokens)
}

func (qs *QuotaStorage) mod(ctx context.Context, names []string, add int64) error {
	now := timeSource.Now()
	return qs.forNames(ctx, names, defaultMode, func(s concurrency.STM, name string, cfg *storagepb.Config) error {
		_, err := modBucket(s, cfg, now, add)
		return err
	})
}

// Peek returns a map of quota name to tokens for the named quotas.
// Unknown or disabled quotas are considered infinite and returned as having quota.MaxTokens tokens,
// therefore all requested names are guaranteed to be in the resulting map
func (qs *QuotaStorage) Peek(ctx context.Context, names []string) (map[string]int64, error) {
	now := timeSource.Now()
	tokens := make(map[string]int64)
	err := qs.forNames(ctx, names, emitInfinite, func(s concurrency.STM, name string, cfg *storagepb.Config) error {
		var err error
		var t int64
		if cfg == nil {
			t = int64(quota.MaxTokens)
		} else {
			t, err = modBucket(s, cfg, now, 0 /* add */)
		}
		tokens[name] = t
		return err
	})
	return tokens, err
}

// Put adds "tokens" tokens to the named quotas.
// Time-based quotas cannot be replenished this way, therefore put requests for them are ignored.
// Unknown or disabled quotas are considered infinite and also ignored.
func (qs *QuotaStorage) Put(ctx context.Context, names []string, tokens int64) error {
	if tokens < 0 {
		return fmt.Errorf("invalid number of tokens: %v", tokens)
	}
	return qs.mod(ctx, names, tokens)
}

// Reset resets the named quotas to their maximum number of tokens.
// Unknown or disabled quotas are considered infinite and ignored.
func (qs *QuotaStorage) Reset(ctx context.Context, names []string) error {
	now := timeSource.Now()
	return qs.forNames(ctx, names, defaultMode, func(s concurrency.STM, name string, cfg *storagepb.Config) error {
		bucket := &storagepb.Bucket{
			Tokens:                        cfg.MaxTokens,
			LastReplenishMillisSinceEpoch: now.UnixNano() / 1e6,
		}
		pb, err := proto.Marshal(bucket)
		if err != nil {
			return err
		}
		s.Put(bucketKey(cfg), string(pb))
		return nil
	})
}

// forNamesMode specifies how forNames handles disabled and infinite quotas.
type forNamesMode int

const (
	// defaultMode emits only known, enabled configs.
	defaultMode forNamesMode = iota

	// emitInfinite emits all known, enabled configs and all infinite configs.
	// Infinite configs are emitted with a nil cfg value.
	// emitInfinite emits disabled configs with a nil cfg value as well.
	emitInfinite
)

// forNames calls fn for all configs specified by names. Execution is performed in a single etcd
// transaction.
// By default, fn is only called for known, enabled configs. See forNamesMode for other behaviors.
// Names are validated and de-duped automatically.
func (qs *QuotaStorage) forNames(ctx context.Context, names []string, mode forNamesMode, fn func(concurrency.STM, string, *storagepb.Config) error) error {
	for _, name := range names {
		if !IsNameValid(name) {
			return fmt.Errorf("invalid name: %q", name)
		}
	}

	_, err := concurrency.NewSTMSerializable(ctx, qs.Client, func(s concurrency.STM) error {
		cfgs, err := getConfigs(s)
		if err != nil {
			return err
		}

		seenNames := make(map[string]bool)
		for _, name := range names {
			if seenNames[name] {
				continue
			}
			seenNames[name] = true

			emitted := false
			for _, cfg := range cfgs.Configs {
				if cfg.Name == name {
					if cfg.State == storagepb.Config_ENABLED {
						if err := fn(s, name, cfg); err != nil {
							return err
						}
						emitted = true
					}
					break
				}
			}
			if !emitted && mode == emitInfinite {
				if err := fn(s, name, nil); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return err
}

func getConfigs(s concurrency.STM) (*storagepb.Configs, error) {
	// TODO(codingllama): Consider watching configs instead of re-reading
	cfgs := &storagepb.Configs{}
	val := s.Get(configsKey)
	if val == "" {
		// Empty value means no config was explicitly created yet.
		// Use the default (empty) configs in this case.
		return cfgs, nil
	}
	if err := proto.Unmarshal([]byte(val), cfgs); err != nil {
		return nil, fmt.Errorf("error unmarshaling %v: %v", configsKey, err)
	}
	return cfgs, nil
}

// modBucket adds "add" tokens to the specified quota. Add may be negative or zero.
// Time-based quotas that are due replenishment will be replenished before the add operation. Quotas
// that are above ceiling (eg, due to lowered max tokens) will also be constrained to the
// appropriate ceiling. As a consequence, calls with add = 0 are still useful for peeking and the
// explained side-effects.
// modBucket returns the current token count for cfg.
func modBucket(s concurrency.STM, cfg *storagepb.Config, now time.Time, add int64) (int64, error) {
	key := bucketKey(cfg)

	val := s.Get(key)
	var prevBucket *storagepb.Bucket
	if err := proto.Unmarshal([]byte(val), prevBucket); err != nil {
		return 0, fmt.Errorf("error unmarshaling %v: %v", key, err)
	}
	newBucket := proto.Clone(prevBucket).(*storagepb.Bucket)

	if tb := cfg.GetTimeBased(); tb != nil {
		if now.Unix() >= newBucket.LastReplenishMillisSinceEpoch/1e3+tb.ReplenishIntervalSeconds {
			newBucket.Tokens += tb.TokensToReplenish
			if newBucket.Tokens > cfg.MaxTokens {
				newBucket.Tokens = cfg.MaxTokens
			}
			newBucket.LastReplenishMillisSinceEpoch = now.UnixNano() / 1e6
		}
		if add > 0 {
			add = 0 // Do not replenish time-based quotas
		}
	}

	newBucket.Tokens += add
	if newBucket.Tokens < 0 {
		return 0, fmt.Errorf("insufficient tokens on %v (%v vs %v)", key, prevBucket.Tokens, -add)
	}
	if newBucket.Tokens > cfg.MaxTokens {
		newBucket.Tokens = cfg.MaxTokens
	}

	if !proto.Equal(prevBucket, newBucket) {
		pb, err := proto.Marshal(newBucket)
		if err != nil {
			return 0, err
		}
		s.Put(key, string(pb))
	}

	return newBucket.Tokens, nil
}

func bucketKey(cfg *storagepb.Config) string {
	return fmt.Sprintf("%v/0", cfg.Name)
}
