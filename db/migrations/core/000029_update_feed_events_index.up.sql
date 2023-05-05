/* {% require_sudo %} */
DROP INDEX IF EXISTS feeds_owner_id_action_event_timestamp_idx;
CREATE INDEX feeds_owner_id_action_event_timestamp_idx ON feed_events (owner_id, action, event_time) WHERE deleted = false;
CREATE INDEX IF NOT EXISTS feeds_global_pagination_idx ON feed_events (event_time desc, id desc) WHERE deleted = false;
CREATE INDEX IF NOT EXISTS feeds_user_pagination_idx ON feed_events (owner_id, event_time desc, id desc) WHERE deleted = false;
