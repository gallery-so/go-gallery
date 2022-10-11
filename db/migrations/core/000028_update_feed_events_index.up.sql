DROP INDEX IF EXISTS feeds_owner_id_action_event_timestamp_idx;
CREATE INDEX feeds_owner_id_action_event_timestamp_idx ON feed_events (owner_id, action, event_time) WHERE deleted = false;
