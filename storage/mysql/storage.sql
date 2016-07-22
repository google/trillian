# MySQL / MariaDB version of the tree schema

-- ---------------------------------------------
-- Tree stuff here
-- ---------------------------------------------


-- Tree parameters should not be changed after creation. Doing so can
-- render the data in the tree unusable or inconsistent.
CREATE TABLE IF NOT EXISTS Trees(
  TreeId                INTEGER NOT NULL,
  KeyId                 VARBINARY(255) NOT NULL,
  TreeType              ENUM('LOG', 'MAP')  NOT NULL,
  LeafHasherType        ENUM('SHA256') NOT NULL,
  TreeHasherType        ENUM('SHA256') NOT NULL,
  AllowsDuplicateLeaves BOOLEAN NOT NULL DEFAULT 0,
  PRIMARY KEY(TreeId)
);

-- This table contains tree parameters that can be changed at runtime such as for
-- administrative purposes.
CREATE TABLE IF NOT EXISTS TreeControl(
  TreeId                  INTEGER NOT NULL,
  ReadOnlyRequests        BOOLEAN,
  SigningEnabled          BOOLEAN,
  SequencingEnabled       BOOLEAN,
  SequenceIntervalSeconds INTEGER,
  SignIntervalSeconds     INTEGER,
  PRIMARY KEY(TreeId),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId)
);

CREATE TABLE IF NOT EXISTS Subtree(
  TreeId               INTEGER NOT NULL,
  SubtreeId            VARBINARY(255) NOT NULL,
  Nodes                VARBINARY(32768) NOT NULL,
  SubtreeRevision      INTEGER NOT NULL,  -- negated because DESC indexes aren't supported :/
  PRIMARY KEY(TreeId, SubtreeId, SubtreeRevision),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE
);

-- The TreeRevisionIdx is used to enforce that there is only one STH at any
-- tree revision
CREATE TABLE IF NOT EXISTS TreeHead(
  TreeId               INTEGER NOT NULL,
  TreeHeadTimestamp    BIGINT,
  TreeSize             BIGINT,
  RootHash             VARBINARY(255) NOT NULL,
  RootSignature        VARBINARY(255) NOT NULL,
  TreeRevision         BIGINT,
  PRIMARY KEY(TreeId, TreeHeadTimestamp),
  UNIQUE INDEX TreeRevisionIdx(TreeId, TreeRevision),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE
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
  TreeId               INTEGER NOT NULL,
  LeafHash             VARBINARY(255) NOT NULL,
  TheData              BLOB NOT NULL,
  PRIMARY KEY(TreeId, LeafHash),
  INDEX LeafHashIdx(LeafHash),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE
);

-- When a leaf is sequenced a row is added to this table. If logs allow duplicates then
-- multiple rows will exist with different sequence numbers. The signed timestamp
-- will be communicated via the unsequenced table as this might need to be unique, depending
-- on the log parameters and we can't insert into this table until we have the sequence number
-- which is not available at the time we queue the entry.
CREATE TABLE IF NOT EXISTS SequencedLeafData(
  TreeId               INTEGER NOT NULL,
  SequenceNumber       BIGINT UNSIGNED NOT NULL,
  LeafHash             VARBINARY(255) NOT NULL,
  SignedEntryTimestamp BLOB NOT NULL,
  PRIMARY KEY(TreeId, SequenceNumber),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE,
  FOREIGN KEY(LeafHash) REFERENCES LeafData(LeafHash)
);

CREATE TABLE IF NOT EXISTS Unsequenced(
  TreeId               INTEGER NOT NULL,
  LeafHash             VARBINARY(255) NOT NULL,
  -- SHA256("queueId"|TreeId|leafHash)
  -- We want this to be unique per entry per log, but queryable by FEs so that
  -- we can try to stomp dupe submissions.
  MessageId            BINARY(32) NOT NULL,
  Payload              BLOB NOT NULL,
  QueueTimestamp       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  SignedEntryTimestamp BLOB,
  PRIMARY KEY (TreeId, LeafHash, MessageId)
);


-- ---------------------------------------------
-- Map specific stuff here
-- ---------------------------------------------

CREATE TABLE IF NOT EXISTS MapLeaf(
  TreeId                INTEGER NOT NULL,
  KeyHash               VARBINARY(255) NOT NULL,
  MapRevision           BIGINT NOT NULL,
  TheData               BLOB NOT NULL,
  PRIMARY KEY(TreeId, KeyHash, MapRevision),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE
);


CREATE TABLE IF NOT EXISTS MapHead(
  TreeId               INTEGER NOT NULL,
  MapHeadTimestamp     BIGINT,
  RootHash             VARBINARY(255) NOT NULL,
  MapRevision          BIGINT,
  RootSignature        VARBINARY(255) NOT NULL,
  TransactionLogRoot   BLOB NOT NULL,
  PRIMARY KEY(TreeId, MapHeadTimestamp),
  UNIQUE INDEX TreeRevisionIdx(TreeId, MapRevision),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE
);

