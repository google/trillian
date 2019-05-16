# MySQL / MariaDB version of the tree schema

-- ---------------------------------------------
-- Tree stuff here
-- ---------------------------------------------

-- Tree parameters should not be changed after creation. Doing so can
-- render the data in the tree unusable or inconsistent.
CREATE TABLE `Trees` (
  `TreeId` bigint(20) NOT NULL,
  `TreeState` enum('ACTIVE','FROZEN','DRAINING') NOT NULL,
  `TreeType` enum('LOG','MAP','PREORDERED_LOG') NOT NULL,
  `HashStrategy` enum('RFC6962_SHA256','TEST_MAP_HASHER','OBJECT_RFC6962_SHA256','CONIKS_SHA512_256','CONIKS_SHA256') NOT NULL,
  `HashAlgorithm` enum('SHA256') NOT NULL,
  `SignatureAlgorithm` enum('ECDSA','RSA') NOT NULL,
  `DisplayName` varchar(20) DEFAULT NULL,
  `Description` varchar(200) DEFAULT NULL,
  `CreateTimeMillis` bigint(20) NOT NULL,
  `UpdateTimeMillis` bigint(20) NOT NULL,
  `MaxRootDurationMillis` bigint(20) NOT NULL,
  `PrivateKey` mediumblob NOT NULL,
  `PublicKey` mediumblob NOT NULL,
  `Deleted` tinyint(1) DEFAULT NULL,
  `DeleteTimeMillis` bigint(20) DEFAULT NULL,
  PRIMARY KEY (`TreeId`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- This table contains tree parameters that can be changed at runtime such as for
-- administrative purposes.
CREATE TABLE `TreeControl` (
  `TreeId` bigint(20) NOT NULL,
  `SigningEnabled` tinyint(1) NOT NULL,
  `SequencingEnabled` tinyint(1) NOT NULL,
  `SequenceIntervalSeconds` int(11) NOT NULL,
  PRIMARY KEY (`TreeId`),
  CONSTRAINT `TreeControl_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `Subtree` (
  `TreeId` bigint(20) NOT NULL,
  `SubtreeId` varbinary(255) NOT NULL,
  `Nodes` mediumblob NOT NULL,
  `SubtreeRevision` int(11) NOT NULL,
  PRIMARY KEY (`TreeId`,`SubtreeId`,`SubtreeRevision`) COMMENT 'Key columns must be in ASC order in order to benefit from group-by/min-max optimization in MySQL.',
  CONSTRAINT `Subtree_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- The TreeRevisionIdx is used to enforce that there is only one STH at any
-- tree revision
CREATE TABLE `TreeHead` (
  `TreeId` bigint(20) NOT NULL,
  `TreeHeadTimestamp` bigint(20) NOT NULL,
  `TreeSize` bigint(20) DEFAULT NULL,
  `RootHash` varbinary(255) NOT NULL,
  `RootSignature` varbinary(1024) NOT NULL,
  `TreeRevision` bigint(20) DEFAULT NULL,
  PRIMARY KEY (`TreeId`,`TreeHeadTimestamp`),
  UNIQUE KEY `TreeHeadRevisionIdx` (`TreeId`,`TreeRevision`),
  CONSTRAINT `TreeHead_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ---------------------------------------------
-- Log specific stuff here
-- ---------------------------------------------

-- Creating index at same time as table allows some storage engines to better
-- optimize physical storage layout. Most engines allow multiple nulls in a
-- unique index but some may not.

-- A leaf that has not been sequenced has a row in this table. If duplicate leaves
-- are allowed they will all reference this row.
CREATE TABLE `LeafData` (
  `TreeId` bigint(20) NOT NULL,
  `LeafIdentityHash` varbinary(255) NOT NULL COMMENT 'This is a personality specific hash of some subset of the leaf data. It''s only purpose is to allow Trillian to identify duplicate entries in the context of the personality.',
  `LeafValue` longblob NOT NULL COMMENT 'This is the data stored in the leaf for example in CT it contains a DER encoded X.509 certificate but is application dependent.',
  `ExtraData` longblob COMMENT 'This is extra data that the application can associate with the leaf should it wish to. This data is not included in signing and hashing.',
  `QueueTimestampNanos` bigint(20) NOT NULL COMMENT 'The timestamp from when this leaf data was first queued for inclusion.',
  PRIMARY KEY (`TreeId`,`LeafIdentityHash`),
  CONSTRAINT `LeafData_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- When a leaf is sequenced a row is added to this table. If logs allow duplicates then
-- multiple rows will exist with different sequence numbers. The signed timestamp
-- will be communicated via the unsequenced table as this might need to be unique, depending
-- on the log parameters and we can't insert into this table until we have the sequence number
-- which is not available at the time we queue the entry. We need both hashes because the
-- LeafData table is keyed by the raw data hash.
CREATE TABLE `SequencedLeafData` (
  `TreeId` bigint(20) NOT NULL,
  `SequenceNumber` bigint(20) unsigned NOT NULL,
  `LeafIdentityHash` varbinary(255) NOT NULL COMMENT 'This is a personality specific has of some subset of the leaf data. It''s only purpose is to allow Trillian to identify duplicate entries in the context of the personality.',
  `MerkleLeafHash` varbinary(255) NOT NULL COMMENT 'This is a MerkleLeafHash as defined by the treehasher that the log uses. For example for CT this hash will include the leaf prefix byte as well as the leaf data.',
  `IntegrateTimestampNanos` bigint(20) NOT NULL,
  PRIMARY KEY (`TreeId`,`SequenceNumber`),
  KEY `TreeId` (`TreeId`,`LeafIdentityHash`),
  KEY `SequencedLeafMerkleIdx` (`TreeId`,`MerkleLeafHash`),
  CONSTRAINT `SequencedLeafData_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE,
  CONSTRAINT `SequencedLeafData_ibfk_2` FOREIGN KEY (`TreeId`, `LeafIdentityHash`) REFERENCES `LeafData` (`TreeId`, `LeafIdentityHash`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


CREATE TABLE `Unsequenced` (
  `TreeId` bigint(20) NOT NULL,
  `Bucket` int(11) NOT NULL COMMENT 'The bucket field is to allow the use of time based ring bucketed schemes if desired. If unused this should be set to zero for all entries.',
  `LeafIdentityHash` varbinary(255) NOT NULL COMMENT 'This is a personality specific hash of some subset of the leaf data. It''s only purpose is to allow Trillian to identify duplicate entries in the context of the personality.',
  `MerkleLeafHash` varbinary(255) NOT NULL COMMENT 'This is a MerkleLeafHash as defined by the treehasher that the log uses. For example for CT this hash will include the leaf prefix byte as well as the leaf data.',
  `QueueTimestampNanos` bigint(20) NOT NULL,
  `QueueID` varbinary(32) DEFAULT NULL COMMENT 'This is a SHA256 hash of the TreeID, LeafIdentityHash and QueueTimestampNanos. It is used for batched deletes from the table when trillian_log_server and trillian_log_signer are built with the batched_queue tag.',
  PRIMARY KEY (`TreeId`,`Bucket`,`QueueTimestampNanos`,`LeafIdentityHash`),
  UNIQUE KEY `QueueID` (`QueueID`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


-- ---------------------------------------------
-- Map specific stuff here
-- ---------------------------------------------

CREATE TABLE `MapLeaf` (
  `TreeId` bigint(20) NOT NULL,
  `KeyHash` varbinary(255) NOT NULL,
  `MapRevision` bigint(20) NOT NULL COMMENT 'MapRevision is stored negated to invert ordering in the primary key index st. more recent revisions come first.',
  `LeafValue` longblob NOT NULL,
  PRIMARY KEY (`TreeId`,`KeyHash`,`MapRevision`),
  CONSTRAINT `MapLeaf_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


CREATE TABLE `MapHead` (
  `TreeId` bigint(20) NOT NULL,
  `MapHeadTimestamp` bigint(20) NOT NULL,
  `RootHash` varbinary(255) NOT NULL,
  `MapRevision` bigint(20) DEFAULT NULL,
  `RootSignature` varbinary(1024) NOT NULL,
  `MapperData` mediumblob,
  PRIMARY KEY (`TreeId`,`MapHeadTimestamp`),
  UNIQUE KEY `MapHeadRevisionIdx` (`TreeId`,`MapRevision`),
  CONSTRAINT `MapHead_ibfk_1` FOREIGN KEY (`TreeId`) REFERENCES `Trees` (`TreeId`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

