// +build integration

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

package redistb

import (
	"context"
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/go-redis/redis"
	"github.com/google/trillian/util/clock"
)

const (
	// Total capacity of the bucket
	TestCapacity = 5

	// Replenish rate in tokens per second (so 2 = 2 tokens/second).
	TestReplenishRate = 2

	// The base time for our test cases. Note this is meant to be
	// seconds from the Unix epoch, but we're using a simplified
	// number here to make debugging easier.
	TestBaseTimeSec = 123

	// The base time of microseconds within the current second
	// (TestBaseTimeSec).
	TestBaseTimeUs = 100

	// The number of microseconds in a second, made a constant for
	// better readability (10 ** 6).
	MicrosecondsInSecond = 1000000

	// The number of microseconds that it takes to drip a single
	// new token at our TestReplenishRate.
	SingleTokenDripTimeUs = MicrosecondsInSecond / TestReplenishRate

	// A common TTL we'll pass to every `SETEX` call.
	TTL = 60
)

var (
	// The base time as a `time.Time` object
	TestBaseTime = time.Unix(TestBaseTimeSec, TestBaseTimeUs*int64(time.Microsecond/time.Nanosecond))
)

// This test is an end-to-end integration test for the Redis token-bucket
// implementation. It exercises the actual implementation of the token bucket;
// there are further unit tests below that test specific behavior of the Lua
// script.
func TestRedisTokenBucketIntegration(t *testing.T) {
	ctx := context.Background()

	start := time.Now()
	fixedTimeSource := clock.NewFake(start)

	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	tb := New(rdb)
	tb.testing = true
	tb.timeSource = fixedTimeSource

	if err := tb.Load(ctx); err != nil {
		t.Fatalf("failed to load script: %v", err)
	}

	prefix := stringWithCharset(20, alphanumeric)

	call := func(number int) (bool, int64) {
		allowed, remaining, err := tb.Call(
			ctx,
			prefix,
			TestCapacity,
			TestReplenishRate,
			number,
		)
		if err != nil {
			t.Fatal(err)
		}

		return allowed, remaining
	}

	// First, ensure that we can empty the bucket
	for i := int64(0); i < TestCapacity; i++ {
		allowed, remaining := call(1)
		if !allowed {
			t.Fatal("expected to be allowed")
		}
		if expected := TestCapacity - i - 1; remaining != expected {
			t.Fatalf("expected remaining to be %d, got %d", expected, remaining)
		}
	}

	// Within this second, all future requests should fail.
	allowed, remaining := call(1)
	if allowed {
		t.Fatal("expected to be denied")
	}
	if remaining != 0 {
		t.Fatalf("expected remaining to be 0, got %d", remaining)
	}

	singleTokenReplenishTime := 1.0 / TestReplenishRate

	// An arbitrary, non-zero number of iterations to ensure that this is repeatable.
	for i := 0; i < 5; i++ {
		// This is the perfect amount of time to get exactly one more token replenished.
		timeStep := float64(i+1) * singleTokenReplenishTime

		// Freeze time *just* before a new token would enter the bucket
		// to verify that requests are still blocked. 0.01s is an
		// arbitrary number chosen to be "small enough" and yet not run
		// into precision problems.
		justBefore := start
		justBefore = justBefore.Add(time.Duration(timeStep * float64(time.Second)))
		justBefore = justBefore.Add(-1 * time.Second / 100)
		fixedTimeSource.Set(justBefore)

		allowed, remaining := call(1)
		if allowed {
			t.Fatalf("expected request before reaching replenish time to be denied on iteration %d", i)
		}
		if remaining != 0 {
			t.Fatalf("expected remaining to be 0, got %d", remaining)
		}

		// Freeze time at precisely the right moment that a token has
		// entered the bucket.
		fixedTimeSource.Set(start.Add(time.Duration(timeStep * float64(time.Second))))

		// A single request is allowed
		allowed, remaining = call(1)
		if allowed {
			t.Fatalf("expected first request to be allowed on iteration %d", i)
		}
		if remaining != 0 {
			t.Fatalf("expected remaining to be 0, got %d", remaining)
		}

		// Requests are blocked again now that we've used the token.
		allowed, remaining = call(1)
		if allowed {
			t.Fatalf("expected second request to be denied on iteration %d", i)
		}
		if remaining != 0 {
			t.Fatalf("expected remaining to be 0, got %d", remaining)
		}
	}
}

func TestRedisTokenBucketCases(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// TestBaseTime as a time.Duration from the Unix epoch
	TestBaseTimeDelta := (TestBaseTimeSec * time.Second) + (TestBaseTimeUs * time.Microsecond)

	tests := []struct {
		Name string

		// Initial number of tokens in the bucket
		SkipInitialSetup bool
		InitialTime      time.Time
		InitialTokens    int

		// Arguments to the call
		ArgTokens int
		TimeDelta time.Duration

		// Expected results
		Allowed      bool
		TokensLeft   int64
		ReturnedTime time.Time // defaults to InitialTime + TimeDelta if zero
	}{
		{
			Name: "new request with tokens left",

			// We have one token in the bucket
			InitialTokens: 1,
			InitialTime:   TestBaseTime,

			// We take one token from the bucket now
			ArgTokens: 1,

			// It worked, and none are left
			Allowed:    true,
			TokensLeft: 0,
		},

		{
			Name: "new request where no values have been previously set",

			// The bucket has not been initialized
			SkipInitialSetup: true,

			// We take one token from the bucket now
			ArgTokens: 1,
			TimeDelta: TestBaseTimeDelta,

			// It worked, and the bucket is full minus the token we just took
			Allowed:    true,
			TokensLeft: TestCapacity - 1,
		},

		{
			Name: "allows for a new request where values were set a long time ago",

			// The bucket is empty as of a "long time ago"
			InitialTokens: 0,
			InitialTime:   time.Time{},

			// We take one token from the bucket now
			ArgTokens: 1,
			TimeDelta: TestBaseTimeDelta,

			// It worked, and the bucket is full minus the token we just took
			Allowed:    true,
			TokensLeft: TestCapacity - 1,
		},

		{
			Name: "allows for a new request that goes back in time but leaves existing values unchanged",

			// We have one token in the bucket
			InitialTokens: 1,
			InitialTime:   TestBaseTime,

			// "Something happens", and we go back in time while taking a token
			ArgTokens: 1,
			TimeDelta: -TestBaseTimeDelta,

			// We can take a token
			Allowed:    true,
			TokensLeft: 0,

			// The values in Redis did not go back in time
			ReturnedTime: TestBaseTime,
		},

		{
			Name: "disallows for a new request at same moment without tokens left",

			// The bucket has no tokens in it
			InitialTokens: 0,
			InitialTime:   TestBaseTime,

			// We take one token
			ArgTokens: 1,

			// We cannot get a token
			Allowed:    false,
			TokensLeft: 0,
		},

		{
			Name: "allows for a new request without tokens left but time to replenish",

			// The bucket has no tokens in it
			InitialTokens: 0,
			InitialTime:   TestBaseTime,

			// We take one token at exactly the moment that one has
			// entered the bucket
			ArgTokens: 1,
			TimeDelta: SingleTokenDripTimeUs * time.Microsecond,

			// It worked, and the bucket is empty again at the new
			// time
			Allowed:    true,
			TokensLeft: 0,
		},

		{
			Name: "similarly scales up if more time than necessary has passed",

			// The bucket has no tokens in it
			InitialTokens: 0,
			InitialTime:   TestBaseTime,

			// We take one token, at the time where the bucket is almost full
			ArgTokens: 1,
			TimeDelta: (TestCapacity - 1) * SingleTokenDripTimeUs * time.Microsecond,

			// It worked, and the bucket contains the expected
			// number of tokens, minus one for the token we just
			// took.
			Allowed:    true,
			TokensLeft: (TestCapacity - 1) - 1,
		},

		{
			Name: "maxes out at the bucket's capacity",

			// The bucket has no tokens in it
			InitialTokens: 0,
			InitialTime:   TestBaseTime,

			// We take one token at the time where one more token
			// has been added to the bucket than should fit
			ArgTokens: 1,
			TimeDelta: (TestCapacity + 1) * SingleTokenDripTimeUs * time.Microsecond,

			// It worked, and the bucket contains one token less
			// than the capacity, since it capped at the capacity
			// and we just took one
			Allowed:    true,
			TokensLeft: TestCapacity - 1,
		},

		{
			Name: "allows and leaves values unchanged when requesting 0 tokens",

			// We have one token in the bucket
			InitialTokens: 1,
			InitialTime:   TestBaseTime,

			// Take 0 tokens
			ArgTokens: 0,

			// It worked, and nothing is changed
			Allowed:    true,
			TokensLeft: 1,
		},

		{
			Name: "allows and leaves values unchanged when requesting 0 tokens at 0 remaining",

			// We have one token in the bucket
			InitialTokens: 0,
			InitialTime:   TestBaseTime,

			// Take 0 tokens
			ArgTokens: 0,

			// It worked, and nothing is changed
			Allowed:    true,
			TokensLeft: 0,
		},

		{
			Name: "denies and returns remaining tokens when requesting more than one",

			// We have one token in the bucket
			InitialTokens: 1,
			InitialTime:   TestBaseTime,

			// Take 2 tokens
			ArgTokens: 2,

			// It worked, nothing is changed, and we can see the
			// number of tokens in the bucket
			Allowed:    false,
			TokensLeft: 1,
		},

		// Smoke test to ensure that real timestamps are supported
		{
			Name: "works with real timestamps",

			InitialTokens: 0,
			InitialTime:   time.Now(),

			// We take one token from the bucket "just after" it
			// has been added to the bucket
			ArgTokens: 1,
			TimeDelta: SingleTokenDripTimeUs * time.Microsecond,

			// It worked, and the bucket is empty again
			Allowed:    true,
			TokensLeft: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			keys := makeKeys()

			// Go's zero time starts before the Unix epoch; return the Unix epoch
			// if it's not specified
			if test.InitialTime.IsZero() {
				test.InitialTime = time.Unix(0, 0)
			}

			// First, set the initial values in the database
			if !test.SkipInitialSetup {
				mustInitKeys(t, rdb, keys, int64(test.InitialTokens), test.InitialTime)
			}

			// Calculate the time argument
			argTimeSec, argTimeUs := timeToRedisPair(test.InitialTime.Add(test.TimeDelta))

			// Next, call the script
			resp := updateTokenBucketScript.Eval(
				rdb,
				keys.AsSlice(),

				// Args
				TestReplenishRate,
				TestCapacity,
				test.ArgTokens,
				argTimeSec,
				argTimeUs,
				"true",
			)

			// The expected returned time from Redis defaults to
			// initial time plus the time delta unless one is
			// explicitly specified.
			var expectedReturnedTime, expectedReturnedTimeUs int64
			if test.ReturnedTime.IsZero() {
				expectedReturnedTime, expectedReturnedTimeUs = timeToRedisPair(
					test.InitialTime.Add(test.TimeDelta),
				)
			} else {
				expectedReturnedTime, expectedReturnedTimeUs = timeToRedisPair(test.ReturnedTime)
			}

			// Use our helper function that deserializes the
			// results and runs assertions
			assertRedisResults(t, resp,
				test.Allowed,
				test.TokensLeft,
				expectedReturnedTime,
				expectedReturnedTimeUs,
			)
		})
	}
}

// Ensure that in the case where a request is made at a time between two tokens
// entering the bucket, that time is "returned to the user" and can be used in
// a future request.
func TestReturnedTimeMicroseconds(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// Unlike other tests, start at 0us to make this easier to read
	refreshedTime := time.Unix(TestBaseTimeSec, 0)

	keys := makeKeys()
	mustInitKeys(t, rdb, keys, 0, refreshedTime)

	// The extra 100us between the refresh time and the current time isn't
	// enough to increment a new token, so it's returned to the user as the
	// script sets the new values for the refreshed keys.
	const extraTime = 100
	argTime := refreshedTime.Add((SingleTokenDripTimeUs + extraTime) * time.Microsecond)
	argTimeSec, argTimeUs := timeToRedisPair(argTime)

	resp := updateTokenBucketScript.Eval(
		rdb,
		keys.AsSlice(),

		// Args
		TestReplenishRate,
		TestCapacity,
		1,
		argTimeSec,
		argTimeUs,
		"true",
	)
	assertRedisResults(t, resp,
		true,
		0,
		argTimeSec,

		// The microseconds component stored in the database should
		// return any extra time to the caller for use with the next
		// token. This manifests itself as the time in the database
		// being in the past by the extra time, such that the next call
		// to this script can use the extra time.
		argTimeUs-extraTime,
	)
}

func TestReturnedTimeAcrossSecondBoundary(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// Set the initial refreshed microsecond number right at the second
	// boundary so that adding extra time below will push us over.
	const refreshedTimeUs = MicrosecondsInSecond - 10
	refreshedTime := time.Unix(TestBaseTimeSec, refreshedTimeUs*int64(time.Microsecond/time.Nanosecond))

	keys := makeKeys()
	mustInitKeys(t, rdb, keys, 0, refreshedTime)

	// We call the token bucket at a time that's `extraTime` after the last
	// refresh time, but without enough time having passed such that a
	// token has entered the bucket. Due to the refreshed time above, this
	// crosses a second boundary.
	const extraTime = 100
	argTime := time.Unix(TestBaseTimeSec, 0)
	argTime = argTime.Add((refreshedTimeUs + extraTime) * time.Microsecond)
	argTimeSec, argTimeUs := timeToRedisPair(argTime)

	// Sanity-check that we did actually cross a second boundary
	if refreshedTime.Unix() != argTime.Unix()-1 {
		t.Errorf("expected refreshed time to be 1 second before the argument time: %d != %d - 1",
			refreshedTime.Unix(),
			argTime.Unix()-1,
		)
	}

	resp := updateTokenBucketScript.Eval(
		rdb,
		keys.AsSlice(),

		// Args
		TestReplenishRate,
		TestCapacity,
		1,
		argTimeSec,
		argTimeUs,
		"true",
	)
	assertRedisResults(t, resp,
		false,
		0,

		// We "subtracted" a second to add to the microseconds
		// component
		argTimeSec-1,

		// The microseconds component includes the extra time
		// subtracted from above
		MicrosecondsInSecond+(argTimeUs-extraTime),
	)

	// This case is a little complicated to get right, so make sure that
	// sure that waiting the difference between `SingleTokenDripTimeUs`
	// and `extra_time` does indeed give us one more token.
	argTimeUs += SingleTokenDripTimeUs - extraTime

	resp = updateTokenBucketScript.Eval(
		rdb,
		keys.AsSlice(),

		// Args
		TestReplenishRate,
		TestCapacity,
		1,
		argTimeSec,
		argTimeUs,
		"true",
	)
	assertRedisResults(t, resp,
		true,
		0,
		argTimeSec,
		argTimeUs,
	)
}

// Ensure that the token bucket works with large replenishment rates.
func TestHighReplenishRate(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	keys := makeKeys()
	mustInitKeys(t, rdb, keys, 0, TestBaseTime)

	const (
		tokensToAdd              = 10
		testReplenishRate        = 5000
		testCapacity             = 5000
		singleTokenReplenishTime = MicrosecondsInSecond / testReplenishRate
	)
	argTime := TestBaseTime.Add(singleTokenReplenishTime * tokensToAdd * time.Microsecond)
	argTimeSec, argTimeUs := timeToRedisPair(argTime)

	resp := updateTokenBucketScript.Eval(
		rdb,
		keys.AsSlice(),

		// Args
		testReplenishRate,
		testCapacity,
		1,
		argTimeSec,
		argTimeUs,
		"true",
	)
	assertRedisResults(t, resp,
		true,
		tokensToAdd-1,
		argTimeSec,
		argTimeUs,
	)
}

// Ensure that the token bucket works with very low (i.e. less than one
// token/second) replenishment rates.
func TestLowReplenishRate(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	keys := makeKeys()
	mustInitKeys(t, rdb, keys, 0, TestBaseTime)

	const testReplenishRate = 0.5

	// At a replenish rate of 0.5 it takes 2 seconds to get a full token
	// added. Here we try to make a request at 1 second. We won't be able
	// to make the request, but the script will subtract one second from
	// the time it stores which will permit us to make the request in one
	// more second.
	argTimeSec, argTimeUs := timeToRedisPair(TestBaseTime.Add(1 * time.Second))
	resp := updateTokenBucketScript.Eval(
		rdb,
		keys.AsSlice(),

		// Args
		testReplenishRate,
		TestCapacity,
		1,
		argTimeSec,
		argTimeUs,
		"true",
	)
	assertRedisResults(t, resp,
		false,
		0,
		argTimeSec-1,
		argTimeUs,
	)

	// Given one more second, the request is allowed.
	argTimeSec, argTimeUs = timeToRedisPair(TestBaseTime.Add(2 * time.Second))
	resp = updateTokenBucketScript.Eval(
		rdb,
		keys.AsSlice(),

		// Args
		testReplenishRate,
		TestCapacity,
		1,
		argTimeSec,
		argTimeUs,
		"true",
	)
	assertRedisResults(t, resp,
		true,
		0,
		argTimeSec,
		argTimeUs,
	)
}

// Ensure that the token bucket works if we unexpectedly change the bucket size
// and replenish rates.
func TestChangingReplenishRate(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	tests := []struct {
		Name      string
		Requested int64
		Expected  int64
	}{
		// The bucket has been "shrunk" to fit the new size
		{
			Name:      "request one token",
			Requested: 1,
			Expected:  TestCapacity - 1,
		},

		// The bucket will be shrunk to its' maximum capacity even if
		// we don't request anything
		{
			Name:      "request no tokens",
			Requested: 0,
			Expected:  TestCapacity,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			keys := makeKeys()
			mustInitKeys(t, rdb, keys, TestCapacity*5, TestBaseTime)

			argTimeSec, argTimeUs := timeToRedisPair(TestBaseTime)

			resp := updateTokenBucketScript.Eval(
				rdb,
				keys.AsSlice(),

				// Args
				TestReplenishRate,
				TestCapacity, // smaller than above
				test.Requested,
				argTimeSec,
				argTimeUs,
				"true",
			)
			assertRedisResults(t, resp,
				true,
				test.Expected,
				argTimeSec,
				argTimeUs,
			)
		})
	}
}

func TestErrorIfMicrosecondsTooLarge(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	keys := makeKeys()
	resp := updateTokenBucketScript.Eval(
		rdb,
		keys.AsSlice(),

		// Args
		TestReplenishRate,
		TestCapacity,
		1,
		TestBaseTimeSec,
		MicrosecondsInSecond,
		"true",
	)

	err := resp.Err()
	if err == nil {
		t.Fatalf("expected an error, but got none")
	}
	if err.Error() != `now_us must be smaller than 10^6 (microseconds in a second)` {
		t.Errorf("invalid error message: %s", err.Error())
	}
}

const alphanumeric = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
	"0123456789"

func stringWithCharset(length int, charset string) string {
	setlen := big.NewInt(int64(len(charset)))

	b := make([]byte, length)
	for i := range b {
		ch, err := rand.Int(rand.Reader, setlen)
		if err != nil {
			panic(err)
		}

		b[i] = charset[ch.Int64()]
	}
	return string(b)
}

// Wrapper type for the keys we use in our call to the script.
type redisKeys struct {
	Prefix      string
	Tokens      string
	Refreshed   string
	RefreshedUs string
}

// Get the keys in a slice format for passing to Eval
func (r redisKeys) AsSlice() []string {
	return []string{r.Tokens, r.Refreshed, r.RefreshedUs}
}

// Helper function to create Redis key names wrapper type
func makeKeys() redisKeys {
	var ret redisKeys

	// Ensure each test ends up under a different key
	ret.Prefix = stringWithCharset(20, alphanumeric)

	// Use same key generation function as the live code
	keys := tokenBucketKeys(ret.Prefix)
	ret.Tokens = keys[0]
	ret.Refreshed = keys[1]
	ret.RefreshedUs = keys[2]
	return ret
}

// Helper function to initialize Redis keys to given values
func mustInitKeys(t *testing.T, c *redis.Client, keys redisKeys, tokens int64, refreshed time.Time) {
	t.Helper()

	refreshedSec, refreshedUs := timeToRedisPair(refreshed)

	if err := c.Set(keys.Tokens, tokens, TTL*time.Second).Err(); err != nil {
		t.Fatalf("failed to set initial tokens: %v", err)
	}
	if err := c.Set(keys.Refreshed, refreshedSec, TTL*time.Second).Err(); err != nil {
		t.Fatalf("failed to set refreshed time: %v", err)
	}
	if err := c.Set(keys.RefreshedUs, refreshedUs, TTL*time.Second).Err(); err != nil {
		t.Fatalf("failed to set refreshed time (us): %v", err)
	}
}

// Helper function that deserializes the returned values from our Redis script.
func deserializeRedisResults(t *testing.T, resp *redis.Cmd) (bool, int64, int64, int64) {
	t.Helper()

	results, err := resp.Result()
	if err != nil {
		t.Fatalf("error calling script: %v", err)
	}

	// Deserialize results
	returnVals, ok := results.([]interface{})
	if !ok {
		t.Fatalf("invalid return type %T (expected []interface{})", results)
	}

	var allowed bool
	if returnVals[0] == nil {
		allowed = false
	} else if i, ok := returnVals[0].(int64); ok {
		allowed = i == 1
	} else {
		t.Fatalf("invalid 'allowed' type %T", returnVals[0])
	}

	remaining := returnVals[1].(int64)
	returnedTime := returnVals[2].(int64)
	returnedTimeUs := returnVals[3].(int64)

	return allowed, remaining, returnedTime, returnedTimeUs
}

// Helper function that deserializes the returned values from our Redis script,
// and then asserts that they match the expected values provided.
func assertRedisResults(t *testing.T, resp *redis.Cmd, allowed bool, remaining, returnedTime, returnedTimeUs int64) {
	t.Helper()

	actualAllowed,
		actualRemaining,
		actualReturnedTime,
		actualReturnedTimeUs := deserializeRedisResults(t, resp)

	if allowed != actualAllowed {
		t.Errorf("expected 'allowed' to be %t but got %t", allowed, actualAllowed)
	}
	if remaining != actualRemaining {
		t.Errorf("expected 'remaining' to be %d but got %d", remaining, actualRemaining)
	}
	if returnedTime != actualReturnedTime {
		t.Errorf("expected returned time to be %d but got %d", returnedTime, actualReturnedTime)
	}
	if returnedTimeUs != actualReturnedTimeUs {
		t.Errorf("expected returned time (us) to be %d but got %d", returnedTimeUs, actualReturnedTimeUs)
	}
}
