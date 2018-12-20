# TRILLIAN Changelog

## HEAD

Not yet released; provisionally v2.0.0 (may change).

### Storage API change

The internal storage API is modified so that the ReadOnlyTreeTX.ReadRevision and
TreeWriter.WriteRevision entrypoints take a context.Context parameter and return
an optional error.

### Maphammer improvements

The maphammer test tool for the experimental Trillian Map has been enhanced.

### Master election refactoring

The `--resign_odds` flag in `logsigner` is removed, in favor of a more generic
`--master_hold_jitter` flag. Operators using this flag are advised to set the
jitter to `master_check_interval * resign_odds * 2` to achieve similar behavior.

The `--master_check_interval` flag is removed from `logsigner`.

`logsigner` switched to using a new master election interface contained in
`util/election2` package. The interfaces in `util/election/election.go` file are
deprecated.

### Other

The `TimeSource` type (and other time utils) moved to a separate `util/clock`
package, extended with a new `Timer` interface that allows mocking `time.Timer`.

## v1.2.1 - Map race fixed. TLS client support. LogClient improvements

Published 2018-08-20 10:31:00 +0000 UTC

### Servers

A race condition was fixed that affected sparse Merkle trees as served by the
map server.

### Utilities / Binaries

The `maphammer` uses a consistent empty check, fixing spurious failures in some
tests.

The `createtree` etc. set of utilities now support TLS via the `-tls-cert-file`
flag. This support is also available as a client module.

### Log Client

`GetAndVerifyInclusionAtIndex` no longer updates the clients root on every
access as this was an unexpected side effect. Clients now have explicit control
of when the root is updated by calling `UpdateRoot`.

A root parameter is now required when log clients are constructed.

### Other

The Travis build script has been updated for newer versions of MySQL (5.7
through MySQL 8) and will no longer work with 5.6.

Commit
[f3eaa887163bb4d2ea4b4458cb4e7c5c2f346bc6](https://api.github.com/repos/google/trillian/commits/f3eaa887163bb4d2ea4b4458cb4e7c5c2f346bc6)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.2.1)

## v1.2.0 - Signer / Quota fixes. Error mapping fix. K8 improvements

Published 2018-06-25 10:42:52 +0000 UTC

The Log Signer now tries to avoid creating roots older than ones that already
exist. This issue has been seen occurring on a test system. Important note: If
running this code in production allowing clocks to drift out of sync between
nodes can cause other problems including for clustering and database
replication.

The Log Signer now publishes metrics for the logs that it is actively signing.
In a clustered environment responsibility can be expected to move around between
signer instances over time.

The Log API now allows personalities to explicitly list a vector of identifiers
which should be charged for `User` quota. This allows a more nuanced application
of request rate limiting across multiple dimensions. Some fixes have also been
made to quota handling e.g. batch requests were not reserving the appropriate
quota. Consult the corresponding PRs for more details.

For the log RPC server APIs `GetLeavesByIndex` and `GetLeavesByRange` MySQL
storage has been modified to return status codes that match CloudSpanner.
Previously some requests with out of range parameters were receiving 5xx error
status rather than 4xx when errors were mapped to the HTTP space by CTFE.

The Kubernetes deployment scripts continue to evolve and improve.

Commit
[aef10347dba1bd86a0fcb152b47989d0b51ba1fa](https://api.github.com/repos/google/trillian/commits/aef10347dba1bd86a0fcb152b47989d0b51ba1fa)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.2.0)

## v1.1.1 - CloudSpanner / Tracing / Health Checks

Published 2018-05-08 12:55:34 +0000 UTC

More improvements have been made to the CloudSpanner storage code. CloudSpanner
storage has now been tested up to ~3.1 billion log entries.

Explicit health checks have been added to the gRPC Log and Map servers (and the
log signer). The HTTP endpoint must be enabled and the checks will serve on
`/healthz` where a non 200 response means the server is unhealthy. The example
Kubernetes deployment configuration has been updated to include them. Other
improvements have been made to the Kubernetes deployment scripts and docs.

The gRPC Log and Map servers have been instrumented for tracing with
[OpenCensus](https://opencensus.io/). For GCP it just requires the `--tracing`
flag to be added and results will be available in the GCP console under
StackDriver -> Trace.

Commit
[3a68a845f0febdd36937c15f1d97a3a0f9509440](https://api.github.com/repos/google/trillian/commits/3a68a845f0febdd36937c15f1d97a3a0f9509440)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.1.1)

## v1.1.0 - CloudSpanner Improvements & Log Root structure changes etc.

Published 2018-04-17 08:02:50 +0000 UTC

Changes are in progress (e.g. see #1037) to rework the internal signed root
format used by the log RPC server to be more useful / interoperable. Currently
they are mostly internal API changes to the log and map servers. However, the
`signature` and `log_id` fields in SignedLogRoot have been deleted and users
must unpack the serialized structure to access these now. This change is not
backwards compatible.

Changes have been made to log server APIs and CT frontends for when a request
hits a server that has an earlier version of the tree than is needed to satisfy
the request. In these cases the log server used to return an error but now
returns an empty proof along with the current STH it has available. This allows
clients to detect these cases and handle them appropriately.

The CloudSpanner schema has changed. If you have a database instance you'll need
to recreate it with the new schema. Performance has been noticeably improved
since the previous release and we have tested it to approx one billion log
entries. Note: This code is still being developed and further changes are
possible.

Support for `sqlite` in unit tests has been removed because of ongoing issues
with flaky tests. These were caused by concurrent accesses to the same database,
which it doesn't support. The use of `sqlite` in production has never been
supported and it should not be used for this.

Commit
[9a5dc6223bab0e1061b66b49757c2418c47b9f29](https://api.github.com/repos/google/trillian/commits/9a5dc6223bab0e1061b66b49757c2418c47b9f29)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.1.0)

## v1.0.8 - Docker Updates / Freezing Logs / CloudSpanner Options

Published 2018-03-08 13:42:11 +0000 UTC

The Docker image files have been updated and the database has been changed to
`MariaDB 10.1`.

A `ReadOnlyStaleness` option has been added to the experimental CloudSpanner
storage. This allows for tuning that might increase performance in some
scenarios by issuing read transactions with the `exact_staleness` option set
rather than `strong_read`. For more details see the
[CloudSpanner TransactionOptions](https://cloud.google.com/spanner/docs/reference/rest/v1/TransactionOptions)
documentation.

The `LogVerifier` interface has been removed from the log client, though the
functionality is still available. It is unlikely that there were implementations
by third-parties.

A new `TreeState DRAINING` has been added for trees with `TreeType LOG`. This is
to support logs being cleanly frozen. A log tree in this state will not accept
new entries via `QueueLeaves` but will continue to integrate any that were
previously queued. When the queue of pending entries has been emptied the tree
can be set to the `FROZEN` state safely. For MySQL storage this requires a
schema update to add `'DRAINING'` to the enum of valid states.

A command line utility `updatetree` has been added to allow tree states to be
changed. This is also to support cleanly freezing logs.

A 'howto' document has been added that explains how to freeze a log tree using
the features added in this release.

Commit
[0e6d950b872d19e42320f4714820f0fe793b9913](https://api.github.com/repos/google/trillian/commits/0e6d950b872d19e42320f4714820f0fe793b9913)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.8)

## v1.0.7 - Storage API Changes, Schema Tweaks

Published 2018-03-01 11:16:32 +0000 UTC

Note: A large number of storage related API changes have been made in this
release. These will probably only affect developers writing their own storage
implementations.

A new tree type `ORDERED_LOG` has been added for upcoming mirror support. This
requires a schema change before it can be used. This change can be made when
convenient and can be deferred until the functionality is available and needed.
The definition of the `TreeType` column enum should be changed to `ENUM('LOG',
'MAP', 'PREORDERED_LOG') NOT NULL`

Some storage interfaces were removed in #977 as they only had one
implementation. We think this won't cause any impact on third parties and are
willing to reconsider this change if it does.

The gRPC Log and Map server APIs have new methods `InitLog` and `InitMap` which
prepare newly created trees for use. Attempting to use trees that have not been
initialized will return the `FAILED_PRECONDITION` error
`storage.ErrTreeNeedsInit`.

The gRPC Log server API has new methods `AddSequencedLeaf` and
`AddSequencedLeaves`. These are intended to support mirroring applications and
are not yet implemented.

Storage APIs have been added such as `ReadWriteTransaction` which allows the
underlying storage to manage the transaction and optionally retry until success
or timeout. This is a more natural fit for some types of storage API such as
[CloudSpanner](https://cloud.google.com/spanner/docs/transactions) and possibly
other environments with managed transactions.

The older `BeginXXX` methods were removed from the APIs. It should be fairly
easy to convert a custom storage implementation to the new API format as can be
seen from the changes made to the MySQL storage.

The `GetOpts` options are no longer used by storage. This fixed the strange
situation of storage code having to pass manufactured dummy instances to
`GetTree`, which was being called in all the layers involved in request
processing. Various internal APIs were modified to take a `*trillian.Tree`
instead of an `int64`.

A new storage implementation has been added for CloudSpanner. This is currently
experimental and does not yet support Map trees. We have also added Docker
examples for running Trillian in Google Cloud with CloudSpanner.

The maximum size of a `VARBINARY` column in MySQL is too small to properly
support Map storage. The type has been changed in the schema to `MEDIUMBLOB`.
This can be done in place with an `ALTER TABLE` command but this could very be
slow for large databases as it is a change to the physical row layout. Note:
There is no need to make this change to the database if you are only using it
for Log storage e.g. for Certificate Transparency servers.

The obsolete programs `queue_leaves` and `fetch_leaves` have been deleted.

Commit
[7d73671537ca2a4745dc94da3dc93d32d7ce91f1](https://api.github.com/repos/google/trillian/commits/7d73671537ca2a4745dc94da3dc93d32d7ce91f1)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.7)

## v1.0.6 - GetLeavesByRange. 403 Permission Errors. Signer Metrics.

Published 2018-02-05 16:00:26 +0000 UTC

A new log server RPC API has been added to get leaves in a range. This is a more
natural fit for CT type applications as it more closely follows the CT HTTP API.

The server now returns 403 for permission denied where it used to return 500
errors. This follows the behaviour of the C++ implementation.

The log signer binary now reports metrics for the number it has signed and the
number of errors that have occurred. This is intended to give more insight into
the state of the queue and integration processing.

Commit
[b20b3109af7b68227c83c5d930271eaa4f0be771](https://api.github.com/repos/google/trillian/commits/b20b3109af7b68227c83c5d930271eaa4f0be771)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.6)

## v1.0.5 - TLS, Merge Delay Metrics, Easier Admin Tests

Published 2018-02-07 09:41:08 +0000 UTC

The API protos have been rebuilt with gRPC 1.3.

Timestamps have been added to the log leaves in the MySQL database. Before
upgrading to this version you **must** make the following schema changes:

*   Add the following column to the `LeafData` table. If you have existing data
    in the queue you might have to remove the NOT NULL clause:
    `QueueTimestampNanos BIGINT NOT NULL`

*   Add the following column to the `SequencedLeafData` table:
    `IntegrateTimestampNanos BIGINT NOT NULL`

The above timestamps are used to export metrics via monitoring that give the
merge delay for each tree that is in use. This is a good metric to use for
alerting on.

The Log and Map RPC servers now support TLS.

AdminServer tests have been improved.

Commit
[dec673baf984c3d22d7b314011d809258ec36821](https://api.github.com/repos/google/trillian/commits/dec673baf984c3d22d7b314011d809258ec36821)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.5)

## v1.0.4 - Fix election issue. Large vendor updates.

Published 2018-02-05 15:42:25 +0000 UTC

An issue has been fixed where the master for a log could resign from the
election while it was in the process of integrating a batch of leaves. We do not
believe this could cause any issues with data integrity because of the versioned
tree storage.

This release includes a large number of vendor commits merged to catch up with
etcd 3.2.10 and gRPC v1.3.

Commit
[1713865ecca0dc8f7b4a8ed830a48ae250fd943b](https://api.github.com/repos/google/trillian/commits/1713865ecca0dc8f7b4a8ed830a48ae250fd943b)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.4)

## v1.0.3 - Auth API. Interceptor fixes. Request validation + More

Published 2018-02-05 15:33:08 +0000 UTC

An authorization API has been added to the interceptors. This is intended for
future development and integration.

Issues where the interceptor would not time out on `PutTokens` have been fixed.
This should make the quota system more robust.

A bug has been fixed where the interceptor did not pass the context deadline
through to other requests it made. This would cause some failing requests to do
so after longer than the deadline with a misleading reason in the log. It did
not cause request failures if they would otherwise succeed.

Metalinter has been added and the code has been cleaned up where appropriate.

Docker and Kubernetes scripts have been available and images are now built with
Go 1.9.

Sqlite has been introduced for unit tests where possible. Note that it is not
multi threaded and cannot support all our testing scenarios. We still require
MySQL for integration tests. Please note that Sqlite **must not** be used for
production deployments as RPC servers are multi threaded database clients.

The Log RPC server now applies tighter validation to request parameters than
before. It's possible that some requests will be rejected. This should not
affect valid requests.

The admin server will only create trees for the log type it is hosted in. For
example the admin server running in the Log server will not create Map trees.
This may be reviewed in future as applications can legitimately use both tree
types.

Commit
[9d08b330ab4270a8e984072076c0b3e84eb4601b](https://api.github.com/repos/google/trillian/commits/9d08b330ab4270a8e984072076c0b3e84eb4601b)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.3)

## v1.0.2 - TreeGC, Go 1.9, Update Private Keys.

Published 2018-02-05 15:18:40 +0000 UTC

Go 1.9 is required.

It is now possible to update private keys via the admin API and this was added
to the available field masks. The key storage format has not changed so we
believe this change is transparent.

Deleted trees are now garbage collected after an interval. This hard deletes
them and they cannot be recovered. Be aware of this before upgrading if you have
any that are in a soft deleted state.

The Admin RPC API has been extended to allow trees to be undeleted - up to the
point where they are hard deleted as set out above.

Commit
[442511ad82108654033c9daa4e72f8a79691dd32](https://api.github.com/repos/google/trillian/commits/442511ad82108654033c9daa4e72f8a79691dd32)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.2)

## v1.0.1 - Batched Queue Option Added

Published 2018-02-05 14:49:33 +0000 UTC

Apart from fixes this release includes the option for a batched queue. This has
been reported to allow faster sequencing but is not enabled by default.

If you want to switch to this you must build the code with the `--tags
batched_queue` option. You must then also apply a schema change if you are
running with a previous version of the database. Add the following column to the
`Unsequenced` table:

`QueueID VARBINARY(32) DEFAULT NULL`

If you don't plan to switch to the `batched_queue` mode then you don't need to
make the above change.

Commit
[afd178f85c963f56ad2ae7d4721d139b1d6050b4](https://api.github.com/repos/google/trillian/commits/afd178f85c963f56ad2ae7d4721d139b1d6050b4)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.1)

## v1.0 - First Log version we believe was ready for use. To support CT.

Published 2018-02-05 13:51:55 +0000 UTC

Quota metrics published. Quota admin api + server implemented. Improvements to
local / AWS deployment. Map fixes and further development. ECDSA key handling
improvements. Key factory improvements. Code coverage added. Quota integration
test added. Etcd quota support in log and map connected up. Incompatibility with
C++ code fixed where consistency proof requests for first == second == 0 were
rejected.

Commit
[a6546d092307f6e0d396068066033b434203824d](https://api.github.com/repos/google/trillian/commits/a6546d092307f6e0d396068066033b434203824d)
Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0)
