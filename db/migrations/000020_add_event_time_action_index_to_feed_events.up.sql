CREATE INDEX IF NOT EXISTS feed_events_event_time_action_owner_id_idx ON feed_events (event_time, action, owner_id) WHERE deleted = false;

-- Remove redundant indexes
DROP INDEX IF EXISTS feeds_event_timestamp_idx;
DROP INDEX IF EXISTS feeds_owner_id_action_event_timestamp_idx;