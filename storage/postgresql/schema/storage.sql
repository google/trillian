-- PostgreSQL version of the tree schema.
--
-- Each statement must end with a semicolon, and there must be a blank line before the next statement.
-- This will ensure that the testdbpgx tokenizer will handle semicolons in the PL/pgSQL functions correctly.

-- ---------------------------------------------
-- Tree stuff here
-- ---------------------------------------------

-- Tree parameters should not be changed after creation. Doing so can
-- render the data in the tree unusable or inconsistent.
CREATE TYPE TreeState AS ENUM ('ACTIVE', 'FROZEN', 'DRAINING');

CREATE TYPE TreeType AS ENUM ('LOG', 'MAP', 'PREORDERED_LOG');


CREATE TABLE IF NOT EXISTS Trees(
  TreeId                BIGINT NOT NULL,
  TreeState             TreeState NOT NULL,
  TreeType              TreeType NOT NULL,
  DisplayName           VARCHAR(20),
  Description           VARCHAR(200),
  CreateTimeMillis      BIGINT NOT NULL,
  UpdateTimeMillis      BIGINT NOT NULL,
  MaxRootDurationMillis BIGINT NOT NULL,
  Deleted               BOOLEAN,
  DeleteTimeMillis      BIGINT,
  PRIMARY KEY(TreeId)
);

-- This table contains tree parameters that can be changed at runtime such as for
-- administrative purposes.
CREATE TABLE IF NOT EXISTS TreeControl(
  TreeId                  BIGINT NOT NULL,
  SigningEnabled          BOOLEAN NOT NULL,
  SequencingEnabled       BOOLEAN NOT NULL,
  SequenceIntervalSeconds INTEGER NOT NULL,
  PRIMARY KEY(TreeId),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS Subtree(
  TreeId               BIGINT NOT NULL,
  SubtreeId            BYTEA NOT NULL,
  Nodes                BYTEA NOT NULL,
  -- Key columns must be in ASC order in order to benefit from group-by/min-max
  -- optimization in PostgreSQL.
  CONSTRAINT Subtree_pk PRIMARY KEY (TreeId, SubtreeId),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE,
  CHECK (length(SubtreeId) <= 255)
);

CREATE TABLE IF NOT EXISTS TreeHead(
  TreeId               BIGINT NOT NULL,
  TreeHeadTimestamp    BIGINT,
  TreeSize             BIGINT,
  RootHash             BYTEA NOT NULL,
  RootSignature        BYTEA NOT NULL,
  PRIMARY KEY(TreeId, TreeHeadTimestamp),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE,
  CHECK (length(RootHash) <= 255),
  CHECK (length(RootSignature) <= 1024)
);

-- ---------------------------------------------
-- Log specific stuff here
-- ---------------------------------------------

-- Creating index at same time as table allows some storage engines to better
-- optimize physical storage layout. Most engines allow multiple nulls in a
-- unique index but some may not.

-- A leaf that has not been sequenced has a row in this table. If duplicate leaves
-- are allowed they will all reference this row.
CREATE TABLE IF NOT EXISTS LeafData(
  TreeId               BIGINT NOT NULL,
  -- This is a personality specific has of some subset of the leaf data.
  -- It's only purpose is to allow Trillian to identify duplicate entries in
  -- the context of the personality.
  LeafIdentityHash     BYTEA NOT NULL,
  -- This is the data stored in the leaf for example in CT it contains a DER encoded
  -- X.509 certificate but is application dependent
  LeafValue            BYTEA NOT NULL,
  -- This is extra data that the application can associate with the leaf should it wish to.
  -- This data is not included in signing and hashing.
  ExtraData            BYTEA,
  -- The timestamp from when this leaf data was first queued for inclusion.
  QueueTimestampNanos  BIGINT NOT NULL,
  PRIMARY KEY(TreeId, LeafIdentityHash),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE,
  CHECK (length(LeafIdentityHash) <= 255)
);

-- When a leaf is sequenced a row is added to this table. If logs allow duplicates then
-- multiple rows will exist with different sequence numbers. The signed timestamp
-- will be communicated via the unsequenced table as this might need to be unique, depending
-- on the log parameters and we can't insert into this table until we have the sequence number
-- which is not available at the time we queue the entry. We need both hashes because the
-- LeafData table is keyed by the raw data hash.
CREATE TABLE IF NOT EXISTS SequencedLeafData(
  TreeId               BIGINT NOT NULL,
  SequenceNumber       BIGINT NOT NULL,
  -- This is a personality specific has of some subset of the leaf data.
  -- It's only purpose is to allow Trillian to identify duplicate entries in
  -- the context of the personality.
  LeafIdentityHash     BYTEA NOT NULL,
  -- This is a MerkleLeafHash as defined by the treehasher that the log uses. For example for
  -- CT this hash will include the leaf prefix byte as well as the leaf data.
  MerkleLeafHash       BYTEA NOT NULL,
  IntegrateTimestampNanos BIGINT NOT NULL,
  PRIMARY KEY(TreeId, SequenceNumber),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE,
  FOREIGN KEY(TreeId, LeafIdentityHash) REFERENCES LeafData(TreeId, LeafIdentityHash) ON DELETE CASCADE,
  CHECK (SequenceNumber >= 0),
  CHECK (length(LeafIdentityHash) <= 255),
  CHECK (length(MerkleLeafHash) <= 255)
);

CREATE INDEX SequencedLeafMerkleIdx
  ON SequencedLeafData(TreeId, MerkleLeafHash);

CREATE INDEX SequencedLeafIdentityIdx
  ON SequencedLeafData(TreeId, LeafIdentityHash);

CREATE TABLE IF NOT EXISTS Unsequenced(
  TreeId               BIGINT NOT NULL,
  -- The bucket field is to allow the use of time based ring bucketed schemes if desired. If
  -- unused this should be set to zero for all entries.
  Bucket               INTEGER NOT NULL,
  -- This is a personality specific hash of some subset of the leaf data.
  -- It's only purpose is to allow Trillian to identify duplicate entries in
  -- the context of the personality.
  LeafIdentityHash     BYTEA NOT NULL,
  -- This is a MerkleLeafHash as defined by the treehasher that the log uses. For example for
  -- CT this hash will include the leaf prefix byte as well as the leaf data.
  MerkleLeafHash       BYTEA NOT NULL,
  QueueTimestampNanos  BIGINT NOT NULL,
  -- This is a SHA256 hash of the TreeId, LeafIdentityHash and QueueTimestampNanos. It is used
  -- for batched deletes from the table.
  QueueID              BYTEA DEFAULT NULL UNIQUE,
  PRIMARY KEY (TreeId, Bucket, QueueTimestampNanos, LeafIdentityHash),
  CHECK (length(LeafIdentityHash) <= 255),
  CHECK (length(MerkleLeafHash) <= 255),
  CHECK (length(QueueID) <= 32)
);

-- Adapted from https://wiki.postgresql.org/wiki/Count_estimate
CREATE OR REPLACE FUNCTION count_estimate(
  table_name text
) RETURNS bigint
LANGUAGE plpgsql AS $$
DECLARE
  n bigint;
  plan jsonb;
BEGIN
  EXECUTE 'SELECT count(1) FROM (SELECT 1 FROM ' || table_name || ' LIMIT 10000) sub' INTO n;
  IF n < 10000 THEN
    RETURN n;
  ELSE
    EXECUTE 'ANALYZE ' || table_name || ';EXPLAIN (FORMAT JSON) SELECT * FROM ' || table_name INTO plan;
    RETURN plan->0->'Plan'->'Plan Rows';
  END IF;
EXCEPTION
  WHEN OTHERS THEN
    RETURN 0;
END;
$$;

CREATE OR REPLACE FUNCTION queue_leaves(
) RETURNS SETOF bytea
LANGUAGE plpgsql AS $$
BEGIN
  LOCK TABLE LeafData, Unsequenced IN SHARE ROW EXCLUSIVE MODE;
  UPDATE TempQueueLeaves t
    SET IsDuplicate = TRUE
    FROM LeafData l
    WHERE t.TreeId = l.TreeId
      AND t.LeafIdentityHash = l.LeafIdentityHash;
  INSERT INTO LeafData (TreeId,LeafIdentityHash,LeafValue,ExtraData,QueueTimestampNanos)
    SELECT TreeId,LeafIdentityHash,LeafValue,ExtraData,QueueTimestampNanos
      FROM TempQueueLeaves
      WHERE NOT IsDuplicate;
  INSERT INTO Unsequenced (TreeId,Bucket,LeafIdentityHash,MerkleLeafHash,QueueTimestampNanos,QueueID)
    SELECT TreeId,0,LeafIdentityHash,MerkleLeafHash,QueueTimestampNanos,QueueID
      FROM TempQueueLeaves
      WHERE NOT IsDuplicate;
  RETURN QUERY SELECT DISTINCT LeafIdentityHash
    FROM TempQueueLeaves
    WHERE IsDuplicate;
END;
$$;

CREATE OR REPLACE FUNCTION add_sequenced_leaves(
) RETURNS TABLE(leaf_identity_hash bytea, is_duplicate_leaf_data boolean, is_duplicate_sequenced_leaf_data boolean)
LANGUAGE plpgsql AS $$
BEGIN
  LOCK TABLE LeafData, SequencedLeafData IN SHARE ROW EXCLUSIVE MODE;
  UPDATE TempAddSequencedLeaves t
    SET IsDuplicateLeafData = TRUE
    FROM LeafData l
    WHERE t.TreeId = l.TreeId
      AND t.LeafIdentityHash = l.LeafIdentityHash;
  UPDATE TempAddSequencedLeaves t
    SET IsDuplicateSequencedLeafData = TRUE
    FROM SequencedLeafData s
    WHERE t.TreeId = s.TreeId
      AND t.SequenceNumber = s.SequenceNumber;
  INSERT INTO LeafData (TreeId,LeafIdentityHash,LeafValue,ExtraData,QueueTimestampNanos)
    SELECT TreeId,LeafIdentityHash,LeafValue,ExtraData,QueueTimestampNanos
      FROM TempAddSequencedLeaves
      WHERE NOT IsDuplicateLeafData
        AND NOT IsDuplicateSequencedLeafData;
  INSERT INTO SequencedLeafData (TreeId,LeafIdentityHash,MerkleLeafHash,SequenceNumber,IntegrateTimestampNanos)
    SELECT TreeId,LeafIdentityHash,MerkleLeafHash,SequenceNumber,0
      FROM TempAddSequencedLeaves
      WHERE NOT IsDuplicateLeafData
        AND NOT IsDuplicateSequencedLeafData;
  RETURN QUERY SELECT LeafIdentityHash, IsDuplicateLeafData, IsDuplicateSequencedLeafData
    FROM TempAddSequencedLeaves;
END;
$$;
