# Etcd quotas

Package etcd (and its subpackages) contain an etcd-based
[quota.Manager](https://github.com/google/trillian/blob/3cf59cdfd0/quota/quota.go#L101)
implementation, with a corresponding REST-based configuration service.

## Usage

First, ensure both `logserver` and `logsigner` are started with the
`--etcd_servers` and `--quota_system=etcd` flags, in addition to other flags.
`logserver` must also be started with a non-empty `--http_endpoint` flag, so the
REST quota API can be bound.

For example:

```bash
trillian_log_server \
  --etcd_servers=... \
  --http_endpoint=localhost:8091 \
  --quota_system=etcd

trillian_log_signer --etcd_servers=... --quota_system=etcd
```

If correctly started, the servers will be using etcd quotas. The default
configuration is empty, which means no quotas are enforced.

The REST quota API may be used to create and update configurations.

For example, the command below creates a sequencing-based, `global/write` quota.
Assuming an expected sequencing performance of 50 QPS, the `max_tokens`
specified below implies a backlog of 4h.

```bash
grpc_cli localhost:8091 v1beta1/quotas/global/write/config <<EOF
{
  "name": "quotas/global/write/config",
  "config": {
    "state": "ENABLED",
    "max_tokens": 288000,
    "sequencing_based": {
    }
  }
}
EOF
```

To list all configured quotas, run:

```bash
grpc_cli localhost:8091 v1beta1/quotas ?view=FULL'
```

Quotas may be retrieved individually or via a series of filters, updated and
deleted through the REST API as well. See
[quotapb.proto](https://github.com/google/trillian/blob/master/quota/etcd/quotapb/quotapb.proto)
for an in-depth description of entities and available methods.

### Maintenance and token exhaustion

During regular system operation, no quota-related maintenance should be
required, as the system should generate at least as many tokens as it spends.

If token exhaustion occurs, there are a few built-in mechanisms that allow for
manual intervention. The question of whether intervention is needed, though, is
an important one and should be answered before any attempts are made to bypass
the system. For example:

* is the `logsigner` working properly and able to keep with the current demand?
* is there a spike in requests that may justify the current token exhaustion?

For "genuine" token exhaustion (i.e. the system really is under a load it can't
cope with), it may be beneficial to let the quota system deny requests until
regular operation is resumed.

That said, the sections below describe actions that may taken to deal with token
exhaustion. All examples use `global/read` as the quota in question; substitute
the name as appropriate.

#### Resetting quotas

Resetting a quota restores its current token count to the configured
`max_tokens` value.

```bash
curl -X PATCH \
  'localhost:8091/v1beta1/quotas/global/read/config?reset_quota=true'
```

#### Disabling quotas

Disabling a quota makes it inactive, effective immediately. Disabled quotas may
be enabled again with a similar update (changing "DISABLED" to "ENABLED").

```bash
curl \
  -d '@-' \
  -s \
  -H 'Content-Type: application/json' \
  -X PATCH \
  'localhost:8091/v1beta1/quotas/global/read/config' <<EOF
{
  "config": {
    "state": "DISABLED"
  },
  "update_mask": ["state"]
}
EOF
```

#### Deleting quotas

Permanently deletes a quota. Consider disabling for a temporary solution.

```bash
curl -X DELETE 'localhost:8091/v1beta1/quotas/global/read/config'
```

### Flags

The following flags apply to etcd quotas:

* [--quota_dry_run](https://github.com/google/trillian/blob/3cf59cdfd0/server/trillian_log_server/main.go#L61)
  (log and map servers)
* [--quota_increase_factor](https://github.com/google/trillian/blob/3cf59cdfd0/server/trillian_log_signer/main.go#L60)
  (logsigner)
* [quota_max_cache_entries](https://github.com/google/trillian/blob/c0a332878f/server/trillian_log_server/main.go#L71)
  (log and map servers)
* [quota_min_batch_size](https://github.com/google/trillian/blob/c0a332878f/server/trillian_log_server/main.go#L69)
  (log and map servers)

`--quota_dry_run`, when set to true, stops quota depletion from blocking
requests. This applies to all quotas, so it's only recommended in early
evaluations of the quota system.

`--quota_increase_factor` is related to token leakage protection. It applies
only to sequencing-based quotas. If `--quota_increase_factor` is 1, each new
leaf sequenced by `logsigner` restores exactly one token. If it's higher than 1,
more tokens are restored per leaf batch. A value slightly higher than 1 (e.g.
1.1) is recommended, so there is some protection against token leakage without
too much compromise of the quota system in exceptional situations.

`--quota_max_cache_entries` and `--quota_min_batch_size` are related to token
caching. Some level of token caching (i.e. both flags having values > 0) is
recommended to lessen the latency impact of rate limiting.

`--quota_min_batch_size` is the minimum number of tokens acquired from etcd. If
a particular request demands fewer tokens than the minimal batch size, the
remaining tokens are kept in memory, potentially saving new requests to etcd
until those are consumed.

`--quota_max_cache_entries` determines how many quota Specs are cached. Tokens
are cached per Spec using a LRU replacement policy. In case of systems with a
high number of trees or users, the least used ones are evicted from the cache
(and their tokens returned).

### Monitoring

The following metrics are relevant when considering quota behavior:

* [interceptor_request_count](https://github.com/google/trillian/blob/3cf59cdfd0/server/interceptor/interceptor.go#L91)
* [interceptor_request_denied_count](https://github.com/google/trillian/blob/3cf59cdfd0/server/interceptor/interceptor.go#L95)
* [quota_acquired_tokens](https://github.com/google/trillian/blob/3cf59cdfd0/quota/metrics.go#L70)
* [quota_returned_tokens](https://github.com/google/trillian/blob/3cf59cdfd0/quota/metrics.go#L71)
* [quota_replenished_tokens](https://github.com/google/trillian/blob/3cf59cdfd0/quota/metrics.go#L71)

Requests denied due to token shortage are labeled on
**interceptor_request_denied_count** as
[insufficient_tokens](https://github.com/google/trillian/blob/3cf59cdfd0/server/interceptor/interceptor.go#L38).
The ratio between **denied_with_insufficient_tokens** and
**interceptor_request_count** is a strong indicator of token exhaustion.

## General concepts

Trillian quotas have a finite number of tokens that get consumed by requests.
Once a quota reaches zero tokens, all requests that would otherwise consume a
token from it will fail with a **resource_exhausted** error. Tokens are
replenished by different mechanisms, depending on the quota configuration (e.g,
X tokens every Y seconds).

Quotas are designed so that a set of quotas, in different levels of granularity,
apply to a single request.

A quota
[Spec](https://github.com/google/trillian/blob/3cf59cdfd0/quota/quota.go#L56)
identifies a particular quota and represents to which requests it applies. Specs
contain a
[Group](https://github.com/google/trillian/blob/3cf59cdfd0/quota/quota.go#L27)
(`global`, `tree` and `user`) and
[Kind](https://github.com/google/trillian/blob/3cf59cdfd0/quota/quota.go#L44)
(`read` or `write`).

A few Spec examples are:

* `global/read` (all read requests)
* `global/write` (all write requests)
* `trees/123/write` (write requests for tree 123)
* `users/alice/read` (read requests made by user "alice")

Each request, depending on whether it's a read or write request, subtracts
tokens from the following Specs:

| read requests  | write requests  |
| -------------- | --------------- |
| users/$id/read | users/$id/write |
| trees/$id/read | trees/$id/write |
| global/read    | global/write    |

Quotas that aren't explicitly configured are considered infinite and won't block
requests.

## Etcd quotas

Etcd quotas implement the concepts described above by storing the quota
configuration and token count in etcd.

Two replenishment mechanisms are available: sequencing-based and time-based.

Sequencing-based replenishment is tied to `logsigner's` progress. A token is
restored for each leaf sequenced from the `Unsequenced` table. As such, it's
only applicable to `global/write` and `trees/write` quotas.

Time-based sequencing replenishes X tokens every Y seconds. It may be applied to
all quotas.

### MMD protection

Sequencing-based quotas may be used as a form of MMD protection. If the number
of write requests accepted by Trillian going beyond the `logsigner's` configured
processing capability, tokens will eventually get exhausted and the system will
fail new write requests with a **resource_exhausted** error. While not ideal,
this helps avoid an eventual MMD loss, which may be a graver offense than
temporary loss of availability.

Both `global/write` and `trees/write` quotas may be used for MMD protection
purposes. It's strongly recommended that `global/write` is set up as a last line
of defense for all systems.

### QPS limits

Time-based quotas effectively work as QPS (queries-per-second) limits (X tokens
in Y seconds is roughly equivalent to X/Y QPS).

All quotas may be configured as time-based, but they may be particularly useful
as per-tree (e.g. limiting test or archival trees) or as per-user.

### Default quotas

Default quotas are pre-configured limits that get automatically applied to new
trees or users.

TODO(codingllama): Default quotas are not yet implemented.

### Quota users

User level quotas are applied to "quota users". Trillian makes no assumptions
about what a quota user is. Therefore, initially, there's a single default user
that is charged for all requests (note that, since no quotas are created by
default, this user charges quotas that are effectively infinite).
