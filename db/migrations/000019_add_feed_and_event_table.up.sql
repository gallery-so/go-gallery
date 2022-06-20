CREATE TABLE IF NOT EXISTS resource_types (
    ID serial PRIMARY KEY,
    RESOURCE_NAME varchar(255),
    DELETED boolean NOT NULL DEFAULT false,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO resource_types (id, resource_name) VALUES
    (0, 'user'),
    (1, 'token'),
    (2, 'collection');

CREATE TABLE IF NOT EXISTS events (
    ID varchar(255) PRIMARY KEY,
    VERSION int NOT NULL DEFAULT 0,
    ACTOR_ID varchar(255) NOT NULL REFERENCES users (id),
    RESOURCE_ID int REFERENCES resource_types (id),
    SUBJECT_ID varchar(255) NOT NULL,
    -- These columns are to maintain referential integrity with each of the resource entitites
    USER_ID varchar(255) REFERENCES users (id),
    TOKEN_ID varchar(255) REFERENCES tokens (id),
    COLLECTION_ID varchar(255) REFERENCES collections (id),
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
