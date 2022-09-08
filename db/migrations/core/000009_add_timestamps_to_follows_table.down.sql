ALTER TABLE follows DROP COLUMN IF EXISTS CREATED_AT;
ALTER TABLE follows DROP COLUMN IF EXISTS LAST_UPDATED;

DROP INDEX IF EXISTS follows_follower_last_updated_idx;
DROP INDEX IF EXISTS follows_followee_last_updated_idx;
