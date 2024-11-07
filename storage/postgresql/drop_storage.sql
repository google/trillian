-- Caution - this removes all tables in our schema

DROP FUNCTION IF EXISTS count_estimate;
DROP FUNCTION IF EXISTS queue_leaves;
DROP FUNCTION IF EXISTS add_sequenced_leaves;

DROP TABLE IF EXISTS Unsequenced;
DROP TABLE IF EXISTS Subtree;
DROP TABLE IF EXISTS SequencedLeafData;
DROP TABLE IF EXISTS TreeHead;
DROP TABLE IF EXISTS LeafData;
DROP TABLE IF EXISTS TreeControl;
DROP TABLE IF EXISTS Trees;

DROP TYPE IF EXISTS TreeType;
DROP TYPE IF EXISTS TreeState;
