CREATE TABLE IF NOT EXISTS user_events (
    ID varchar(255) PRIMARY KEY,
    USER_ID varchar(255),
    VERSION int,
    EVENT_TYPE int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DATA JSONB
);

CREATE INDEX IF NOT EXISTS user_id_event_type_created_at ON user_events (USER_ID, EVENT_TYPE, CREATED_AT);

CREATE TABLE IF NOT EXISTS token_events (
    ID varchar(255) PRIMARY KEY,
    USER_ID varchar(255),
    TOKEN_ID varchar(255),
    VERSION int,
    EVENT_TYPE int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DATA JSONB
);

CREATE INDEX IF NOT EXISTS user_id_token_id_event_type_created_at ON token_events (USER_ID, TOKEN_ID, EVENT_TYPE, CREATED_AT);

CREATE TABLE IF NOT EXISTS collection_events (
    ID varchar(255) PRIMARY KEY,
    USER_ID varchar(255),
    COLLECTION_ID varchar(255),
    VERSION int,
    EVENT_TYPE int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DATA JSONB
);

CREATE INDEX IF NOT EXISTS user_id_collection_id_event_type_created_at ON collection_events (USER_ID, COLLECTION_ID, EVENT_TYPE, CREATED_AT);
