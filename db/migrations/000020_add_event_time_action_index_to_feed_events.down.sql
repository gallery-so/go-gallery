DROP INDEX IF EXISTS feed_events_event_time_action_owner_id_idx;
CREATE INDEX IF NOT EXISTS feeds_event_timestamp_idx ON feed_events (event_time);
CREATE INDEX IF NOT EXISTS feeds_owner_id_action_event_timestamp_idx ON feed_events (owner_id, action, event_time);