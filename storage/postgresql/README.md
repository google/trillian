# PostgreSQL storage implementation

## Origin

This storage implementation, added in [PR #3644](https://github.com/google/trillian/pull/3644), began as a fork of the MySQL storage implementation after [PR #3201](https://github.com/google/trillian/pull/3201) was merged.

## Motivation

Sectigo operates Certificate Transparency logs that run Trillian with MariaDB, using the MySQL storage implementation. One log's MariaDB database [suffered unrecoverable corruption](https://groups.google.com/a/chromium.org/g/ct-policy/c/038B7F4g8cU/m/KsOJaEhnBgAJ) as a result of disk space exhaustion, and another log has [struggled to sequence entries quickly enough](https://groups.google.com/a/chromium.org/g/ct-policy/c/wVhEWVI7Xzo/m/0WiIEbZ_BgAJ). Sectigo has more experience with PostgreSQL, believes that PostgreSQL databases are not vulnerable to corruption due to disk space exhaustion, and anticipates that PostgreSQL can achieve significantly greater sequencing throughput.

## Database driver

The [pgx](https://github.com/jackc/pgx) driver is used directly. This offers faster performance than the standard `database/sql` interface, and provides access to a number of PostgreSQL-specific features such as `COPY`.

## Major changes compared to the MySQL storage implementation

- Implemented [bulk processing](#bulk-processing) to greatly improve performance, making use of `COPY`, temporary tables, and database functions.
- Switched to [batched queuing](https://github.com/google/trillian/pull/717), for further performance gains.
- Removed SQL statement caching, because pgx does this itself automatically.
- Removed several vestigial features (e.g., pre-#3201 subtree revisions).
- Forked `storage/testdb` to `storage/postgresql/testdbpgx`, because the former only supports the `database/sql` interface.

## Bulk processing

The `QueueLeaves`, `AddSequencedLeaves`, `UpdateSequencedLeaves`, and `storeSubtrees` functions all operate on sets of records. The individual INSERT statements inherited from the MySQL storage implementation have been replaced by the use of PostgreSQL's `COPY` interface, which bulk-loads data more efficiently and in far fewer network round trips.

`QueueLeaves`, `AddSequencedLeaves`, and `storeSubtrees` each bulk-load data into temporary tables that are bound to a single transaction. This approach enables each function to perform its processing efficiently, after which the processed data is written to the real tables.

`QueueLeaves` and `AddSequencedLeaves` each use a corresponding PL/pgSQL function to perform multiple processing steps involving the temporary tables, which includes the leaf deduplication logic. This could all instead have been implemented as multiple SQL statements called from the Go code, but the approach taken reduces the number of network round trips and the amount of data being transferred to and from the database, and therefore improves performance.

`AddSequencedLeaves` avoids having to use (and to sometimes rollback) savepoints, which further improves performance compared to the equivalent MySQL implementation.
