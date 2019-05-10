-- Postgres impl of storage
-- ---------------------------------------------
-- Tree stuff here
-- ---------------------------------------------

-- Tree Enums
CREATE TYPE E_TREE_STATE AS ENUM('ACTIVE', 'FROZEN', 'DRAINING');--end
CREATE TYPE E_TREE_TYPE AS ENUM('LOG', 'MAP', 'PREORDERED_LOG');--end
CREATE TYPE E_HASH_STRATEGY AS ENUM('RFC6962_SHA256', 'TEST_MAP_HASHER', 'OBJECT_RFC6962_SHA256', 'CONIKS_SHA512_256', 'CONIKS_SHA256');--end
CREATE TYPE E_HASH_ALGORITHM AS ENUM('SHA256');--end
CREATE TYPE E_SIGNATURE_ALGORITHM AS ENUM('ECDSA', 'RSA');--end

-- Tree parameters should not be changed after creation. Doing so can
-- render the data in the tree unusable or inconsistent.
CREATE TABLE IF NOT EXISTS trees (
  tree_id                  BIGINT NOT NULL,
  tree_state               E_TREE_STATE NOT NULL,
  tree_type                E_TREE_TYPE NOT NULL,
  hash_strategy            E_HASH_STRATEGY NOT NULL,
  hash_algorithm           E_HASH_ALGORITHM NOT NULL,
  signature_algorithm      E_SIGNATURE_ALGORITHM NOT NULL,
  display_name             VARCHAR(20),
  description              VARCHAR(200),
  create_time_millis       BIGINT NOT NULL,
  update_time_millis       BIGINT NOT NULL,
  max_root_duration_millis BIGINT NOT NULL,
  private_key              BYTEA NOT NULL,
  public_key               BYTEA NOT NULL,
  deleted                  BOOLEAN NOT NULL DEFAULT FALSE,
  delete_time_millis       BIGINT,
  current_tree_data	   json,
  root_signature	   BYTEA,
  PRIMARY KEY(tree_id)
);--end

-- This table contains tree parameters that can be changed at runtime such as for
-- administrative purposes.
CREATE TABLE IF NOT EXISTS tree_control(
  tree_id                   BIGINT NOT NULL,
  signing_enabled           BOOLEAN NOT NULL,
  sequencing_enabled        BOOLEAN NOT NULL,
  sequence_interval_seconds INTEGER NOT NULL,
  PRIMARY KEY(tree_id),
  FOREIGN KEY(tree_id) REFERENCES trees(tree_id) ON DELETE CASCADE
);--end

CREATE TABLE IF NOT EXISTS subtree(
  tree_id               BIGINT NOT NULL,
  subtree_id            BYTEA NOT NULL,
  nodes                 BYTEA NOT NULL,
  subtree_revision      INTEGER NOT NULL,
  PRIMARY KEY(tree_id, subtree_id, subtree_revision),
  FOREIGN KEY(tree_id) REFERENCES Trees(tree_id) ON DELETE CASCADE
);--end

-- The TreeRevisionIdx is used to enforce that there is only one STH at any
-- tree revision
CREATE TABLE IF NOT EXISTS tree_head(
  tree_id                BIGINT NOT NULL,
  tree_head_timestamp    BIGINT,
  tree_size              BIGINT,
  root_hash              BYTEA NOT NULL,
  root_signature         BYTEA NOT NULL,
  tree_revision          BIGINT,
  PRIMARY KEY(tree_id, tree_revision),
  FOREIGN KEY(tree_id) REFERENCES trees(tree_id) ON DELETE CASCADE
);--end

-- TODO(vishal) benchmark this to see if it's a suitable replacement for not
-- having a DESC scan on the primary key
CREATE UNIQUE INDEX TreeHeadRevisionIdx ON tree_head(tree_id, tree_revision DESC);--end

-- ---------------------------------------------
-- Log specific stuff here
-- ---------------------------------------------

-- Creating index at same time as table allows some storage engines to better
-- optimize physical storage layout. Most engines allow multiple nulls in a
-- unique index but some may not.

-- A leaf that has not been sequenced has a row in this table. If duplicate leaves
-- are allowed they will all reference this row.
CREATE TABLE IF NOT EXISTS leaf_data(
  tree_id               BIGINT NOT NULL,
  -- This is a personality specific hash of some subset of the leaf data.
  -- It's only purpose is to allow Trillian to identify duplicate entries in
  -- the context of the personality.
  leaf_identity_hash     BYTEA NOT NULL,
  -- This is the data stored in the leaf for example in CT it contains a DER encoded
  -- X.509 certificate but is application dependent
  leaf_value            BYTEA NOT NULL,
  -- This is extra data that the application can associate with the leaf should it wish to.
  -- This data is not included in signing and hashing.
  extra_data            BYTEA,
  -- The timestamp from when this leaf data was first queued for inclusion.
  queue_timestamp_nanos  BIGINT NOT NULL,
  PRIMARY KEY(tree_id, leaf_identity_hash),
  FOREIGN KEY(tree_id) REFERENCES trees(tree_id) ON DELETE CASCADE
);--end

-- When a leaf is sequenced a row is added to this table. If logs allow duplicates then
-- multiple rows will exist with different sequence numbers. The signed timestamp
-- will be communicated via the unsequenced table as this might need to be unique, depending
-- on the log parameters and we can't insert into this table until we have the sequence number
-- which is not available at the time we queue the entry. We need both hashes because the
-- LeafData table is keyed by the raw data hash.
CREATE TABLE IF NOT EXISTS sequenced_leaf_data(
  tree_id                   BIGINT NOT NULL,
  sequence_number           BIGINT NOT NULL,
  -- This is a personality specific has of some subset of the leaf data.
  -- It's only purpose is to allow Trillian to identify duplicate entries in
  -- the context of the personality.
  leaf_identity_hash        BYTEA NOT NULL,
  -- This is a MerkleLeafHash as defined by the treehasher that the log uses. For example for
  -- CT this hash will include the leaf prefix byte as well as the leaf data.
  merkle_leaf_hash          BYTEA NOT NULL,
  integrate_timestamp_nanos BIGINT NOT NULL,
  PRIMARY KEY(tree_id,sequence_number),
  FOREIGN KEY(tree_id) REFERENCES trees(tree_id) ON DELETE CASCADE,
  FOREIGN KEY(tree_id, leaf_identity_hash) REFERENCES leaf_data(tree_id, leaf_identity_hash) ON DELETE CASCADE
);--end

CREATE INDEX SequencedLeafMerkleIdx ON sequenced_leaf_data(tree_id, merkle_leaf_hash);--end

CREATE TABLE IF NOT EXISTS unsequenced(
  tree_id               BIGINT NOT NULL,
  -- The bucket field is to allow the use of time based ring bucketed schemes if desired. If
  -- unused this should be set to zero for all entries.
  bucket                INTEGER NOT NULL,
  -- This is a personality specific hash of some subset of the leaf data.
  -- It's only purpose is to allow Trillian to identify duplicate entries in
  -- the context of the personality.
  leaf_identity_hash    BYTEA NOT NULL,
  -- This is a MerkleLeafHash as defined by the treehasher that the log uses. For example for
  -- CT this hash will include the leaf prefix byte as well as the leaf data.
  merkle_leaf_hash      BYTEA NOT NULL,
  queue_timestamp_nanos BIGINT NOT NULL,
  -- This is a SHA256 hash of the TreeID, LeafIdentityHash and QueueTimestampNanos. It is used
  -- for batched deletes from the table when trillian_log_server and trillian_log_signer are
  -- built with the batched_queue tag.
  queue_id              BYTEA DEFAULT NULL UNIQUE,
  PRIMARY KEY (tree_id, bucket, queue_timestamp_nanos, leaf_identity_hash)
);--end

CREATE OR REPLACE FUNCTION public.insert_leaf_data_ignore_duplicates(tree_id bigint, leaf_identity_hash bytea, leaf_value bytea, extra_data bytea, queue_timestamp_nanos bigint)
 RETURNS boolean
 LANGUAGE plpgsql
AS $function$
    begin
        INSERT INTO leaf_data(tree_id,leaf_identity_hash,leaf_value,extra_data,queue_timestamp_nanos) VALUES (tree_id,leaf_identity_hash,leaf_value,extra_data,queue_timestamp_nanos);
	return true;
    exception
        when unique_violation then
		return false;
        when others then
                raise notice '% %', SQLERRM, SQLSTATE;
    end;
$function$;--end

CREATE OR REPLACE FUNCTION public.insert_leaf_data_ignore_duplicates(tree_id bigint, leaf_identity_hash bytea, merkle_leaf_hash bytea, queue_timestamp_nanos bigint)
 RETURNS boolean
 LANGUAGE plpgsql
AS $function$
    begin
        INSERT INTO unsequenced(tree_id,bucket,leaf_identity_hash,merkle_leaf_hash,queue_timestamp_nanos) VALUES(tree_id,0,leaf_identity_hash,merkle_leaf_hash,queue_timestamp_nanos);
        return true;
    exception
        when unique_violation then
                return false;
        when others then
                raise notice '% %', SQLERRM, SQLSTATE;
    end;
$function$;--end



CREATE OR REPLACE FUNCTION public.insert_sequenced_leaf_data_ignore_duplicates(tree_id bigint, sequence_number bigint, leaf_identity_hash bytea, merkle_leaf_hash bytea, integrate_timestamp_nanos bigint)
 RETURNS boolean
 LANGUAGE plpgsql
AS $function$
    begin
       INSERT INTO sequenced_leaf_data(tree_id, sequence_number, leaf_identity_hash, merkle_leaf_hash, integrate_timestamp_nanos) VALUES(tree_id, sequence_number, leaf_identity_hash, merkle_leaf_hash, integrate_timestamp_nanos);
	return true;
    exception
        when unique_violation then
                return false;
        when others then
                raise notice '% %', SQLERRM, SQLSTATE;
    end;
$function$;--end

