CREATE OR ALTER FUNCTION ignore_duplicates(tree_id bigint, leaf_identity_hash bytea, leaf_value bytea, extra_data bytea, queue_timestamp_nanos bigint)
 RETURNS bit
LANGUAGE plpgsql
AS $function$
    begin
        INSERT INTO leaf_data(tree_id,leaf_identity_hash,leaf_value,extra_data,queue_timestamp_nanos) VALUES (tree_id,leaf_identity_hash,leaf_value,extra_data,queue_timestamp_nanos);
        return 1;
    exception
        when unique_violation then
                return 0;
        when others then
                raise notice '% %', SQLERRM, SQLSTATE;
    end;
$function$

CREATE OR ALTER FUNCTION ignore_duplicates(tree_id bigint, leaf_identity_hash bytea, merkle_leaf_hash bytea, queue_timestamp_nanos bigint)
 RETURNS bit
 LANGUAGE plpgsql
AS $function$
    begin
        INSERT INTO unsequenced(tree_id,bucket,leaf_identity_hash,merkle_leaf_hash,queue_timestamp_nanos) VALUES(tree_id,0,leaf_identity_hash,merkle_leaf_hash,queue_timestamp_nanos);
        return 1;
    exception when others then
      return 0;
    end;
$function$

