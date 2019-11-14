--[[

TOKEN BUCKET
===================

Script to read and update a token bucket maintained in Redis. This is an
implementation of the token bucket algorithm which is a common fixture seen in
rate limiting:

    https://en.wikipedia.org/wiki/Token_bucket

For each key prefix, we maintain three values:

    * `<prefix>.tokens`: Number of tokens in bucket at refresh time.

    * `<prefix>.refreshed`: Time in epoch seconds when this prefix's bucket was
      last updated.

    * `<prefix>.refreshed_us`: The microsecond component of the last updated
      time above. Stored separately because a Unix epoch with a microsecond
      component brushes up uncomfortably close to integer boundaries.

The basic strategy is to, at update/read time, fill in all tokens
that would have accumulated since the last update, and then if
possible deduct the number of requested tokens (or disallow the
requested action if there are not enough tokens).

The approach relies on the atomicity of EVAL in redis - only 1 command (EVAL or
otherwise) will be running concurrently per shard in the Redis cluster. Redis
and Lua are very fast, so in practice this works out okay.

A note on units: all times (instants) are measured in epoch seconds with a
separate microsecond component, durations in imicroseconds, and rates in
tokens/second (e.g., a rate of 100 is 100 tokens/second).

For debugging, I'd recommend adding Redis log statements and then tailing your
Redis log. Example:

    redis.log(redis.LOG_WARNING, string.format("rate = %s", rate))

--]]

--
-- Constants
--
-- Lua doesn't actually have constants, so these are constants by convention
-- only. Please don't modify them.
--

local MICROSECONDS_IN_SECOND = 1000000.0

--
-- Functions
--

local function subtract_time (base, base_us, leftover_time_us)
    base = base - math.floor(leftover_time_us / MICROSECONDS_IN_SECOND)

    leftover_time_us = leftover_time_us % MICROSECONDS_IN_SECOND

    base_us = base_us - leftover_time_us
    if base_us < 0 then
        base = base - 1
        base_us = MICROSECONDS_IN_SECOND + base_us
    end

    return base, base_us
end

--
-- Keys and arguments
--

local key_tokens = KEYS[1]

-- Unix time since the epoch in microseconds runs up uncomfortably close to
-- integer boundaries, so we store time as two separate components: (1) seconds
-- since epoch, and (2) microseconds with the current second.
local key_refreshed = KEYS[2]
local key_refreshed_us = KEYS[3]

local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local requested = tonumber(ARGV[3])

-- Callers are allowed to inject the current time into the script, but note
-- that outside of testing, this will always superseded by the time reported by
-- the Redis instance so as to protect against clock drift on any particular
-- local node.
local now = tonumber(ARGV[4])
local now_us = tonumber(ARGV[5])

-- This is ugly, but all values passed in from Ruby get converted to strings
local testing = ARGV[6] == "true"

--
-- Program body
--

-- See comment above.
if testing then
    if now_us >= MICROSECONDS_IN_SECOND then
        return redis.error_reply("now_us must be smaller than 10^6 (microseconds in a second)")
    end
else
    -- Scripts in Redis are pure functions by default which allows Redis to
    -- replicate the entire script rather than the individual commands that it
    -- contains. Because we're about to invoke `TIME` which produces a
    -- non-deterministic result, we need to tell Redis to instead switch to
    -- command-level replication for write operations. It will error if we
    -- don't.
    redis.replicate_commands()

    local current_time = redis.call("TIME")

    -- Redis `TIME` comes back in two components: (1) seconds since epoch, and
    -- (2) microseconds within the current second.
    now = tonumber(current_time[1])
    now_us = tonumber(current_time[2])
end

local filled_tokens = capacity

local last_tokens = redis.call("GET", key_tokens)

local last_refreshed = redis.call("GET", key_refreshed)

local last_refreshed_us = redis.call("GET", key_refreshed_us)

-- Only bother performing rate calculations if we actually need to. i.e., The
-- user has made a request recently enough to still be in the system.
if last_tokens and last_refreshed then
    last_tokens = tonumber(last_tokens)
    last_refreshed = tonumber(last_refreshed)

    -- Rejected a `now` that reads before our recorded `last_refreshed` time.
    -- No reversed deltas are allowed.
    if now < last_refreshed then
        now = last_refreshed
        now_us = last_refreshed_us
    end

    local delta = now - last_refreshed
    local delta_us = delta * MICROSECONDS_IN_SECOND + (now_us - last_refreshed_us)

    -- The time (in microseconds) that it takes to "drip" a single token. For
    -- example, if our rate is 100 tokens per second, then one token is allowed
    -- every 10^6 / 100 = 10,000 microseconds.
    local single_token_time_us = math.floor(MICROSECONDS_IN_SECOND / rate)

    local new_tokens = math.floor(delta_us / single_token_time_us)
    filled_tokens = math.min(capacity, last_tokens + new_tokens)

    -- For maximum fairness, modify the last refresh time by any leftover time
    -- that didn't go towards adding a token.
    --
    -- However, only bother with this if the bucket hasn't been replenished to
    -- full capacity. If it was, the user has had more replenishment time than
    -- they can use anyway.
    if filled_tokens ~= capacity then
        local leftover_time_us = delta_us % single_token_time_us
        now, now_us = subtract_time(now, now_us, leftover_time_us)
    end
end

local allowed = filled_tokens >= requested
local new_tokens = filled_tokens
if allowed then
    new_tokens = filled_tokens - requested
end

-- Set a TTL on the values we set in Redis that will expire them after the
-- point in time they would have been fully replenished, which allows us to
-- manage space more efficiently by removing keys that don't need to be in
-- there.
--
-- Keys that are ~always in use because their owners make frequent requests
-- will be updated by this script constantly (which sets new TTLs), and
-- never expire.
local fill_time = math.ceil(capacity / rate)
local ttl = math.floor(fill_time * 2)

-- Redis will reject a expiry of 0 to `SETEX`, so make sure TTL is always at
-- least 1.
ttl = math.max(ttl, 1)

-- In our tests we freeze time. Because we can't freeze Redis' notion of time
-- and want to make sure that keys we set within test cases don't expire, we
-- forego the standard TTL that we would have set for just a long one to make
-- sure anything we set expires well after the test case will have finished.
if testing then
    ttl = 3600
end

redis.call("SETEX", key_tokens, ttl, new_tokens)
redis.call("SETEX", key_refreshed, ttl, now)
redis.call("SETEX", key_refreshed_us, ttl, now_us)

return { allowed, new_tokens, now, now_us }
