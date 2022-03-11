CREATE TABLE IF NOT EXISTS user_events (
    ID varchar(255) PRIMARY KEY,
    USER_ID varchar(255),
    VERSION int,
    EVENT_CODE int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DATA JSONB
);

CREATE INDEX IF NOT EXISTS user_id_event_code_created_at ON user_events (USER_ID, EVENT_CODE, CREATED_AT);

CREATE TABLE IF NOT EXISTS nft_events (
    ID varchar(255) PRIMARY KEY,
    USER_ID varchar(255),
    NFT_ID varchar(255),
    VERSION int,
    EVENT_CODE int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DATA JSONB
);

CREATE INDEX IF NOT EXISTS user_id_nft_id_event_code_created_at ON nft_events (USER_ID, NFT_ID, EVENT_CODE, CREATED_AT);

CREATE TABLE IF NOT EXISTS collection_events (
    ID varchar(255) PRIMARY KEY,
    USER_ID varchar(255),
    COLLECTION_ID varchar(255),
    VERSION int,
    EVENT_CODE int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DATA JSONB
);

CREATE INDEX IF NOT EXISTS user_id_collection_id_event_code_created_at ON collection_events (USER_ID, COLLECTION_ID, EVENT_CODE, CREATED_AT);
