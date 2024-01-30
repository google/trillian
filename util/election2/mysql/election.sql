-- MySQL / MariaDB version of the leader election schema

-- We only have a single table called LeaderElection.  It contains
-- a row holding the current leader for each resource, as well as the
-- timestamp that the election was acquired at (last_update).
--
-- This is less an election than a mad scramble at the start, but once
-- a leader has won the election, they remain in power until they
-- resign or fail to update the last_update time for 10x the
-- electionInterval, which should be coordinated across participants.
-- This is extremely simple, and doesn't perform any sort of
-- load-shedding or fairness at this layer.
CREATE TABLE IF NOT EXISTS LeaderElection(
    resource_id VARCHAR(50) PRIMARY KEY,
    leader VARCHAR(300) NOT NULL,
    last_update TIMESTAMP NOT NULL
);