# MySQL / MariaDB version of the tree schema

-- ---------------------------------------------
-- Tree stuff here
-- ---------------------------------------------

-- Enable strict mode, so invalid data on inserts/updates is treated as error
-- instead of warning.
-- https://dev.mysql.com/doc/refman/5.7/en/sql-mode.html#sql-mode-strict
SET GLOBAL sql_mode = 'STRICT_ALL_TABLES';

-- Tree parameters should not be changed after creation. Doing so can
-- render the data in the tree unusable or inconsistent.
CREATE TABLE IF NOT EXISTS Trees(
  TreeId                BIGINT NOT NULL,
  TreeState             ENUM('ACTIVE', 'FROZEN', 'SOFT_DELETED', 'HARD_DELETED') NOT NULL,
  TreeType              ENUM('LOG', 'MAP') NOT NULL,
  HashStrategy          ENUM('RFC_6962') NOT NULL,
  HashAlgorithm         ENUM('SHA256') NOT NULL,
  SignatureAlgorithm    ENUM('ECDSA', 'RSA') NOT NULL,
  DuplicatePolicy       ENUM('NOT_ALLOWED', 'ALLOWED') NOT NULL,
  DisplayName           VARCHAR(20),
  Description           VARCHAR(200),
  CreateTime            DATETIME NOT NULL,
  UpdateTime            DATETIME NOT NULL,
  PrivateKey            BLOB NOT NULL,
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
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId)
);

CREATE TABLE IF NOT EXISTS Subtree(
  TreeId               BIGINT NOT NULL,
  SubtreeId            VARBINARY(255) NOT NULL,
  Nodes                VARBINARY(32768) NOT NULL,
  SubtreeRevision      INTEGER NOT NULL,  -- negated because DESC indexes aren't supported :/
  PRIMARY KEY(TreeId, SubtreeId, SubtreeRevision),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE
);

-- The TreeRevisionIdx is used to enforce that there is only one STH at any
-- tree revision
CREATE TABLE IF NOT EXISTS TreeHead(
  TreeId               BIGINT NOT NULL,
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
  TreeId               BIGINT NOT NULL,
  -- This is a personality specific has of some subset of the leaf data.
  -- It's only purpose is to allow Trillian to identify duplicate entries in
  -- the context of the personality.
  LeafIdentityHash     VARBINARY(255) NOT NULL,
  -- This is the data stored in the leaf for example in CT it contains a DER encoded
  -- X.509 certificate but is application dependent
  LeafValue            BLOB NOT NULL,
  -- This is extra data that the application can associate with the leaf should it wish to.
  -- This data is not included in signing and hashing.
  ExtraData            BLOB,
  PRIMARY KEY(TreeId, LeafIdentityHash),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE
);

-- When a leaf is sequenced a row is added to this table. If logs allow duplicates then
-- multiple rows will exist with different sequence numbers. The signed timestamp
-- will be communicated via the unsequenced table as this might need to be unique, depending
-- on the log parameters and we can't insert into this table until we have the sequence number
-- which is not available at the time we queue the entry. We need both hashes because the
-- LeafData table is keyed by the raw data hash.
CREATE TABLE IF NOT EXISTS SequencedLeafData(
  TreeId               BIGINT NOT NULL,
  SequenceNumber       BIGINT UNSIGNED NOT NULL,
  -- This is a personality specific has of some subset of the leaf data.
  -- It's only purpose is to allow Trillian to identify duplicate entries in
  -- the context of the personality.
  LeafIdentityHash     VARBINARY(255) NOT NULL,
  -- This is a MerkleLeafHash as defined by the treehasher that the log uses. For example for
  -- CT this hash will include the leaf prefix byte as well as the leaf data.
  MerkleLeafHash       VARBINARY(255) NOT NULL,
  PRIMARY KEY(TreeId, SequenceNumber),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE,
  FOREIGN KEY(TreeId, LeafIdentityHash) REFERENCES LeafData(TreeId, LeafIdentityHash) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS Unsequenced(
  TreeId               BIGINT NOT NULL,
  -- This is a personality specific has of some subset of the leaf data.
  -- It's only purpose is to allow Trillian to identify duplicate entries in
  -- the context of the personality.
  LeafIdentityHash     VARBINARY(255) NOT NULL,
  -- This is a MerkleLeafHash as defined by the treehasher that the log uses. For example for
  -- CT this hash will include the leaf prefix byte as well as the leaf data.
  MerkleLeafHash       VARBINARY(255) NOT NULL,
  -- SHA256("queueId"|TreeId|leafValueHash)
  -- We want this to be unique per entry per log, but queryable by FEs so that
  -- we can try to stomp dupe submissions.
  MessageId            BINARY(32) NOT NULL,
  QueueTimestampNanos  BIGINT NOT NULL,
  PRIMARY KEY (TreeId, LeafIdentityHash, MessageId)
);


-- ---------------------------------------------
-- Map specific stuff here
-- ---------------------------------------------

CREATE TABLE IF NOT EXISTS MapLeaf(
  TreeId                BIGINT NOT NULL,
  KeyHash               VARBINARY(255) NOT NULL,
  -- MapRevision is stored negated to invert ordering in the primary key index
  -- st. more recent revisions come first.
  MapRevision           BIGINT NOT NULL,
  LeafValue             BLOB NOT NULL,
  PRIMARY KEY(TreeId, KeyHash, MapRevision),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE
);


CREATE TABLE IF NOT EXISTS MapHead(
  TreeId               BIGINT NOT NULL,
  MapHeadTimestamp     BIGINT,
  RootHash             VARBINARY(255) NOT NULL,
  MapRevision          BIGINT,
  RootSignature        VARBINARY(255) NOT NULL,
  MapperData           BLOB,
  PRIMARY KEY(TreeId, MapHeadTimestamp),
  UNIQUE INDEX TreeRevisionIdx(TreeId, MapRevision),
  FOREIGN KEY(TreeId) REFERENCES Trees(TreeId) ON DELETE CASCADE
);

