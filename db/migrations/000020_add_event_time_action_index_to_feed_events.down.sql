DROP INDEX IF EXISTS feed_events_event_time_action_idx;
CREATE INDEX IF NOT EXISTS feeds_event_timestamp_idx ON feed_events (event_time);