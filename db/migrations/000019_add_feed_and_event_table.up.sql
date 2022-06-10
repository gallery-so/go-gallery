CREATE TABLE IF NOT EXISTS events (
    ID varchar(255) PRIMARY KEY,
    VERSION int NOT NULL DEFAULT 0,
    ACTOR_ID varchar(255) NOT NULL REFERENCES users (id),
    SUBJECT_ID varchar(255) NOT NULL,
    ACTION varchar(255) NOT NULL,
    DATA JSONB,
    DELETED boolean NOT NULL DEFAULT false,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS events_actor_id_action_created_at_idx ON events (actor_id, action, created_at DESC);

CREATE TABLE IF NOT EXISTS feed_events (
    ID varchar(255) PRIMARY KEY,
    VERSION int NOT NULL DEFAULT 0,
    OWNER_ID varchar(255) NOT NULL REFERENCES users (ID),
    ACTION varchar(255) NOT NULL,
    DATA JSONB,
    EVENT_TIME timestamptz NOT NULL,
    EVENT_IDS varchar(255)[],
    DELETED boolean NOT NULL DEFAULT false,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS feeds_event_timestamp_idx ON feed_events (event_time DESC);
CREATE INDEX IF NOT EXISTS feeds_owner_id_action_event_timestamp_idx ON feed_events (owner_id, action, event_time DESC);
