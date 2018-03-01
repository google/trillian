# TRILLIAN Changelog

## v1.0.7 - Storage API Changes, Schema Tweaks

Published 2018-03-01 11:16:32 +0000 UTC

Note: A large number of storage related API changes have been made in this release. These will probably only affect developers writing their own storage implementations.

A new tree type `ORDERED_LOG` has been added for upcoming mirror support. This requires a schema change before it can be used. This change can be made when convenient and can be deferred until the functionality is available and needed. The definition of the `TreeType` column enum should be changed to `ENUM('LOG', 'MAP', 'PREORDERED_LOG') NOT NULL`

Some storage interfaces were removed in #977 as they only had one implementation. We think this won't cause any impact on third parties and are willing to reconsider this change if it does.

The gRPC Log and Map server APIs have new methods `InitLog` and `InitMap` which prepare newly created trees for use. Attempting to use trees that have not been initialized will return the `FAILED_PRECONDITION` error `storage.ErrTreeNeedsInit`.

The gRPC Log server API has new methods `AddSequencedLeaf` and `AddSequencedLeaves`. These are intended to support mirroring applications and are not yet implemented.

Storage APIs have been added such as `ReadWriteTransaction` which allows the underlying storage to manage the transaction and optionally retry until success or timeout. This is a more natural fit for some types of storage API such as [CloudSpanner](https://cloud.google.com/spanner/docs/transactions) and possibly other environments with managed transactions. 

The older `BeginXXX` methods were removed from the APIs. It should be fairly easy to convert a custom storage implementation to the new API format as can be seen from the changes made to the MySQL storage.

The `GetOpts` options are no longer used by storage. This fixed the strange situation of storage code having to pass manufactured dummy instances to `GetTree`, which was being called in all the layers involved in request processing. Various internal APIs were modified to take a `*trillian.Tree` instead of an `int64`.

A new storage implementation has been added for CloudSpanner. This is currently experimental and does not yet support Map trees. We have also added Docker examples for running Trillian in Google Cloud with CloudSpanner.

The maximum size of a `VARBINARY` column in MySQL is too small to properly support Map storage. The type has been changed in the schema to `MEDIUMBLOB`. This can be done in place with an `ALTER TABLE` command but this could very be slow for large databases as it is a change to the physical row layout. Note: There is no need to make this change to the database if you are only using it for Log storage e.g. for Certificate Transparency servers.

The obsolete programs `queue_leaves` and `fetch_leaves` have been deleted.

Commit [7d73671537ca2a4745dc94da3dc93d32d7ce91f1](https://api.github.com/repos/google/trillian/commits/7d73671537ca2a4745dc94da3dc93d32d7ce91f1) Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.7)

## v1.0.6 - GetLeavesByRange. 403 Permission Errors. Signer Metrics.

Published 2018-02-05 16:00:26 +0000 UTC

A new log server RPC API has been added to get leaves in a range. This is a more natural fit for CT type applications as it more closely follows the CT HTTP API.

The server now returns 403 for permission denied where it used to return 500 errors. This follows the behaviour of the C++ implementation.

The log signer binary now reports metrics for the number it has signed and the number of errors that have occurred. This is intended to give more insight into the state of the queue and integration processing.

Commit [b20b3109af7b68227c83c5d930271eaa4f0be771](https://api.github.com/repos/google/trillian/commits/b20b3109af7b68227c83c5d930271eaa4f0be771) Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.6)

## v1.0.5 - TLS, Merge Delay Metrics, Easier Admin Tests

Published 2018-02-07 09:41:08 +0000 UTC

The API protos have been rebuilt with gRPC 1.3.

Timestamps have been added to the log leaves in the MySQL database. Before upgrading to this version you **must** make the following schema changes:

* Add the following column to the `LeafData` table. If you have existing data in the queue you might have to remove the NOT NULL clause: `QueueTimestampNanos  BIGINT NOT NULL`

* Add the following column to the `SequencedLeafData` table: `IntegrateTimestampNanos BIGINT NOT NULL`

The above timestamps are used to export metrics via monitoring that give the merge delay for each tree that is in use. This is a good metric to use for alerting on.

The Log and Map RPC servers now support TLS. 

AdminServer tests have been improved.


Commit [dec673baf984c3d22d7b314011d809258ec36821](https://api.github.com/repos/google/trillian/commits/dec673baf984c3d22d7b314011d809258ec36821) Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.5)

## v1.0.4 - Fix election issue. Large vendor updates.

Published 2018-02-05 15:42:25 +0000 UTC

An issue has been fixed where the master for a log could resign from the election while it was in the process of integrating a batch of leaves. We do not believe this could cause any issues with data integrity because of the versioned tree storage.

This release includes a large number of vendor commits merged to catch up with etcd 3.2.10 and gRPC v1.3.


Commit [1713865ecca0dc8f7b4a8ed830a48ae250fd943b](https://api.github.com/repos/google/trillian/commits/1713865ecca0dc8f7b4a8ed830a48ae250fd943b) Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.4)

## v1.0.3 - Auth API. Interceptor fixes. Request validation + More

Published 2018-02-05 15:33:08 +0000 UTC

An authorization API has been added to the interceptors. This is intended for future development and integration.

Issues where the interceptor would not time out on `PutTokens` have been fixed. This should make the quota system more robust.

A bug has been fixed where the interceptor did not pass the context deadline through to other requests it made. This would cause some failing requests to do so after longer than the deadline with a misleading reason in the log. It did not cause request failures if they would otherwise succeed.

Metalinter has been added and the code has been cleaned up where appropriate.

Docker and Kubernetes scripts have been available and images are now built with Go 1.9.

Sqlite has been introduced for unit tests where possible. Note that it is not multi threaded and cannot support all our testing scenarios. We still require MySQL for integration tests. Please note that Sqlite **must not** be used for production deployments as RPC servers are multi threaded database clients.

The Log RPC server now applies tighter validation to request parameters than before. It's possible that some requests will be rejected. This should not affect valid requests.

The admin server will only create trees for the log type it is hosted in. For example the admin server running in the Log server will not create Map trees. This may be reviewed in future as applications can legitimately use both tree types.


Commit [9d08b330ab4270a8e984072076c0b3e84eb4601b](https://api.github.com/repos/google/trillian/commits/9d08b330ab4270a8e984072076c0b3e84eb4601b) Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.3)

## v1.0.2 - TreeGC, Go 1.9, Update Private Keys.

Published 2018-02-05 15:18:40 +0000 UTC

Go 1.9 is required.

It is now possible to update private keys via the admin API and this was added to the available field masks. The key storage format has not changed so we believe this change is transparent.

Deleted trees are now garbage collected after an interval. This hard deletes them and they cannot be recovered. Be aware of this before upgrading if you have any that are in a soft deleted state.

The Admin RPC API has been extended to allow trees to be undeleted - up to the point where they are  hard deleted as set out above.

Commit [442511ad82108654033c9daa4e72f8a79691dd32](https://api.github.com/repos/google/trillian/commits/442511ad82108654033c9daa4e72f8a79691dd32) Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.2)

## v1.0.1 - Batched Queue Option Added

Published 2018-02-05 14:49:33 +0000 UTC

Apart from fixes this release includes the option for a batched queue. This has been reported to allow faster sequencing but is not enabled by default.

If you want to switch to this you must build the code with the `--tags batched_queue` option. You must then also apply a schema change if you are running with a previous version of the database.  Add the following column to the `Unsequenced` table:

`QueueID VARBINARY(32) DEFAULT NULL`

If you don't plan to switch to the `batched_queue` mode then you don't need to make the above change.

Commit [afd178f85c963f56ad2ae7d4721d139b1d6050b4](https://api.github.com/repos/google/trillian/commits/afd178f85c963f56ad2ae7d4721d139b1d6050b4) Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0.1)

## v1.0 - First Log version we believe was ready for use. To support CT.

Published 2018-02-05 13:51:55 +0000 UTC

Quota metrics published. Quota admin api + server implemented. Improvements to local / AWS deployment. Map fixes and further development. ECDSA key handling improvements. Key factory improvements. Code coverage added. Quota integration test added. Etcd quota support in log and map connected up. Incompatibility with C++ code fixed where consistency proof requests for first == second == 0 were rejected.

Commit [a6546d092307f6e0d396068066033b434203824d](https://api.github.com/repos/google/trillian/commits/a6546d092307f6e0d396068066033b434203824d) Download [zip](https://api.github.com/repos/google/trillian/zipball/v1.0)

