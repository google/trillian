# Etcd quotas

Package etcd (and its subpackages) contain an etcd-based
[quota.Manager](https://github.com/google/trillian/blob/3cf59cdfd07fb6245d492efa6ce7c2f309a445ac/quota/quota.go#L101)
implementation, with a corresponding REST-based configuration service.

## Usage

First, ensure both logserver and logsigner are started with the `--etcd_servers`
and `--quota_system=etcd` flags, in addition to other flags. Logserver must
also be started with a non-empty `--http_endpoint` flag, so the REST quota API
is bound.

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

For example, the command below creates a sequencing-based, global/write quota.
Assuming an expected sequencing performance of 50 QPS, the max_tokens specified
implies a backlog of 4h.

```bash
curl \
  -d '@-' \
  -s \
  -H 'Content-Type: application/json' \
  -X POST \
  "localhost:8091/v1beta1/quotas/global/write/config" <<EOF
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

To list all configured quotas, do:

```bash
curl localhost:8091/v1beta1/quotas?view=FULL
```

Quotas may be retrieved individually or via a series of filters, updated and
deleted through the REST API as well. See
[quotapb.proto](https://github.com/google/trillian/blob/master/quota/etcd/quotapb/quotapb.proto)
for an in-depth description of entities and available methods.

## General concepts

Trillian quotas have a finite number of tokens that get consumed (subtracted) by
requests. Once a quota reaches zero tokens, all requests that would otherwise
consume a token from it will fail with a **resource_exhausted** error. Tokens
are replenished (added) by different mechanisms, depending on the quota
configuration (e.g, X tokens every Y seconds).

Quotas are designed so that a set of quotas, in different levels of granularity,
apply to a single request.

A quota
[Spec](https://github.com/google/trillian/blob/3cf59cdfd07fb6245d492efa6ce7c2f309a445ac/quota/quota.go#L56)
identifies a particular quota and represents to which requests its token count
applies to. Specs contain a
[Group](https://github.com/google/trillian/blob/3cf59cdfd07fb6245d492efa6ce7c2f309a445ac/quota/quota.go#L27)
(global, tree and user) and
[Kind](https://github.com/google/trillian/blob/3cf59cdfd07fb6245d492efa6ce7c2f309a445ac/quota/quota.go#L44)
(read or write).

A few Spec examples are:

* global/read (all read requests)
* global/write (all write requests)
* trees/123/write (write requests for tree 123)
* users/alice/read (read requests made by user "alice")

Each request, depending on whether it's a read or write request, subtracts
tokens from the following Specs:

* users/$id/read or users/$id/write
* trees/$id/read or trees/$id/write
* global/read or global/write

Quotas that aren't explicitly configured are considered infinite, thus they'll
never block requests.

## Etcd quotas

Etcd quotas implement the concepts described above by storing the quota
configuration and token count in etcd.

Two replenishment mechanisms are available: sequencing-based and time-based.

Sequencing-based replenishment is tied to logsigner's progress. A token is
restored for each leaf sequenced from the Unsequenced table. As such, it's only
applicable to global/write and tree/write quotas.

Time-based sequencing replenishes X tokens every Y seconds. It may be applied to
all quotas.

### MMD protection

Sequencing-based quotas may be used as a form of MMD protection. That is, if the
number of write requests received by Trillian are beyond logsigner's processing
capabilities, tokens will eventually get exhausted and the system will fail new
write requests with a **resource_exhausted** error. While not ideal, this helps
avoid an eventual MMD loss, which may be a graver offense than temporary loss of
availability.

Both global/write and tree/write quotas may be used for MMD protection purposes.
It's strongly recommended that global/write is set up as a last line of defense
for all systems.

### QPS limits

Time-based quotas effectively work as QPS (queries-per-second) limits (X tokens
in Y seconds is roughly equivalent to X/Y QPS).

All quotas may be configured as time-based, but they may be particularly useful
as per-tree (e.g., limiting test or archival trees) or as per-user.

### Default quotas

Default quotas are pre-configured limits that get automatically applied to new
trees or users.

TODO(codingllama): Default quotas are not yet implemented.

### Quota users

User level quotas are applied to "quota users". Trillian makes no assumptions
about what a quota user is. Therefore, initially, there's a single default user
that is charged for all requests (note that, since no quotas are created by
default, this user charges quotas that are effectively infinite).

TODO(codingllama): Allow personalities to specify the quota user to be charged.
As of this moment, the only way to specify users would be to fork Trillian or
make a PR.
