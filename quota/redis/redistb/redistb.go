// Copyright 2017 Google LLC. All Rights Reserved.
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

// Package redistb implements a token bucket using Redis.
package redistb

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis"
	"github.com/google/trillian/util/clock"
)

// RedisClient is an interface that encompasses the various methods used by
// TokenBucket, and allows selecting among different Redis client
// implementations (e.g. regular Redis, Redis Cluster, sharded, etc.)
type RedisClient interface {
	// Required to load and execute scripts
	Eval(script string, keys []string, args ...interface{}) *redis.Cmd
	EvalSha(sha1 string, keys []string, args ...interface{}) *redis.Cmd
	ScriptExists(hashes ...string) *redis.BoolSliceCmd
	ScriptLoad(script string) *redis.StringCmd
}

// TokenBucket implements a token-bucket limiter stored in a Redis database. It
// supports atomic operation with concurrent access.
type TokenBucket struct {
	c RedisClient

	testing    bool
	timeSource clock.TimeSource
}

// New returns a new TokenBucket that uses the provided Redis client.
func New(client RedisClient) *TokenBucket {
	ret := &TokenBucket{
		c:          client,
		timeSource: clock.System,
	}
	return ret
}

// Load preloads any required Lua scripts into the Redis database, and updates
// the hash of the resulting script. Calling this function is optional, but
// will greatly reduce the network traffic to the Redis cluster since it only
// needs to pass a hash of the script and not the full script content.
func (tb *TokenBucket) Load(ctx context.Context) error {
	client := withClientContext(ctx, tb.c)
	return updateTokenBucketScript.Load(client).Err()
}

// Reset resets the token bucket for the given prefix.
func (tb *TokenBucket) Reset(ctx context.Context, prefix string) error {
	client := withClientContext(ctx, tb.c)

	// Use `EVAL` so that deleting all keys is atomic.
	resp := client.Eval(
		`redis.call("del", KEYS[1]); redis.call("del", KEYS[2]); redis.call("del", KEYS[3])`,
		tokenBucketKeys(prefix),
	)
	return resp.Err()
}

// Call implements the actual token bucket algorithm. Given a bucket with
// capacity `capacity` and replenishment rate of `replenishRate` tokens per
// second, it will first ensure that the bucket has the correct number of
// tokens added (up to the maximum capacity) since the last time that this
// function was called. Then, it will attempt to remove `numTokens` from the
// bucket.
//
// This function returns a boolean indicating whether it was able to remove all
// tokens from the bucket, the remaining number of tokens in the bucket, and
// any error that occurs.
func (tb *TokenBucket) Call(
	ctx context.Context,
	prefix string,
	capacity int64,
	replenishRate float64,
	numTokens int,
) (bool, int64, error) {
	client := withClientContext(ctx, tb.c)

	var (
		now   int64
		nowUs int64
	)
	if tb.testing {
		now, nowUs = timeToRedisPair(tb.timeSource.Now())
	}

	args := []interface{}{
		replenishRate,
		capacity,
		numTokens,

		// The script allows us to inject the current time for testing,
		// but it's superseded by Redis's time in production to protect
		// against clock drift.
		now,
		nowUs,
		tb.testing,
	}

	resp := updateTokenBucketScript.Run(
		client,
		tokenBucketKeys(prefix),
		args...,
	)
	result, err := resp.Result()
	if err != nil {
		return false, 0, err
	}

	returnVals, ok := result.([]interface{})
	if !ok {
		return false, 0, fmt.Errorf("redistb: invalid return type %T (expected []interface{})", result)
	}

	// The script returns:
	//    allowed       Whether the operation was allowed
	//    remaining     The remaining tokens in the bucket
	//    now_new       The script's view of the current time
	//    now_new_us    The script's view of the current time (microseconds)
	//
	// We don't use the last two arguments here.

	// Deserializing turns Lua 'true' into '1', and 'false' into 'nil'
	var allowed bool
	if returnVals[0] == nil {
		allowed = false
	} else if i, ok := returnVals[0].(int64); ok {
		allowed = i == 1
	} else {
		return false, 0, fmt.Errorf("redistb: invalid 'allowed' type %T", returnVals[0])
	}

	remaining := returnVals[1].(int64)
	return allowed, remaining, nil
}

// tokenBucketKeys returns the keys used for the token bucket script, given a
// prefix.
func tokenBucketKeys(prefix string) []string {
	// Redis Cluster uses a hashing algorithm on keys to determine which slot
	// they map to in its backend. Normally this is a problem for EVAL/EVALSHA
	// because multiple keys in a script will likely map to different slots
	// and cause Redis Cluster to reject the request.
	//
	// It's addressed with the idea of a "hash tag":
	//
	// https://redis.io/topics/cluster-tutorial#redis-cluster-data-sharding
	//
	// If a key name contains a string inside of "{}" then _just_ that string
	// is hashed to be used slotting purposes, thereby giving users some
	// control over mapping consistently to certain slots. For example,
	// `this{foo}key` and `another{foo}key` are guaranteed to both map to the
	// same slot.
	//
	// We take advantage of this idea here by making sure to hash only the
	// common identifier in these keys by using "{}".
	return []string{
		fmt.Sprintf("{%s}.tokens", prefix),
		fmt.Sprintf("{%s}.refreshed", prefix),
		fmt.Sprintf("{%s}.refreshed_us", prefix),
	}
}

// timeToRedisPair converts a Go time.Time into a seconds and microseconds
// component, which can be passed to our Redis script.
func timeToRedisPair(t time.Time) (int64, int64) {
	// The first number in the pair is the number of seconds since the Unix
	// epoch.
	timeSec := t.Unix()

	// The second number is any additional number of microseconds; we can
	// get this by obtaining any sub-second Nanoseconds and simply dividing
	// to get the number in microseconds.
	timeMicros := int64(t.Nanosecond()) / int64(time.Microsecond)

	return timeSec, timeMicros
}

// Because each Redis client type in the Go package has a `WithContext` method
// that returns a concrete type, we can't simply put that method in the
// RedisClient interface. This method performs type assertions to try and call
// the `WithContext` method on the appropriate concrete type.
func withClientContext(ctx context.Context, client RedisClient) RedisClient {
	type withContextable interface {
		WithContext(context.Context) RedisClient
	}

	switch c := client.(type) {
	// The three major Redis clients
	case *redis.Client:
		return c.WithContext(ctx)
	case *redis.ClusterClient:
		return c.WithContext(ctx)
	case *redis.Ring:
		return c.WithContext(ctx)

	// Let's also support the case where someone implements a custom client
	// that returns the RedisClient interface type (e.g. good for tests).
	case withContextable:
		return c.WithContext(ctx)
	}

	// If we can't determine a type, just return it unchanged.
	return client
}
