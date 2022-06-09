CREATE SCHEMA IF NOT EXISTS public;

CREATE TABLE IF NOT EXISTS users (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    USERNAME varchar(255),
    USERNAME_IDEMPOTENT varchar(255),
    ADDRESSES varchar(255) [],
    BIO varchar
);

CREATE TABLE IF NOT EXISTS galleries (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    VERSION int,
    OWNER_USER_ID varchar(255),
    COLLECTIONS varchar(255) []
);

CREATE TABLE IF NOT EXISTS nfts (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    NAME varchar,
    DESCRIPTION varchar,
    COLLECTORS_NOTE varchar,
    EXTERNAL_URL varchar,
    CREATOR_ADDRESS varchar(255),
    CREATOR_NAME varchar,
    OWNER_ADDRESS varchar(255),
    MULTIPLE_OWNERS boolean,
    CONTRACT jsonb,
    OPENSEA_ID bigint,
    OPENSEA_TOKEN_ID varchar(255),
    TOKEN_COLLECTION_NAME varchar,
    IMAGE_URL varchar,
    IMAGE_THUMBNAIL_URL varchar,
    IMAGE_PREVIEW_URL varchar,
    IMAGE_ORIGINAL_URL varchar,
    ANIMATION_URL varchar,
    ANIMATION_ORIGINAL_URL varchar,
    ACQUISITION_DATE varchar,
    TOKEN_METADATA_URL varchar
);

CREATE UNIQUE INDEX IF NOT EXISTS opensea_id_owner_address_inx ON nfts (OPENSEA_ID, OWNER_ADDRESS)
WHERE
    NOT DELETED;

CREATE TABLE IF NOT EXISTS collections (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    OWNER_USER_ID varchar(255),
    NFTS varchar(255) [],
    VERSION int,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    HIDDEN boolean NOT NULL DEFAULT false,
    COLLECTORS_NOTE varchar,
    NAME varchar(255),
    LAYOUT jsonb
);

CREATE TABLE IF NOT EXISTS collections_v2 (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    OWNER_USER_ID varchar(255),
    NFTS varchar(255) [],
    VERSION int,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    HIDDEN boolean NOT NULL DEFAULT false,
    COLLECTORS_NOTE varchar,
    NAME varchar(255),
    LAYOUT jsonb
);

CREATE TABLE IF NOT EXISTS nonces (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    USER_ID varchar(255),
    ADDRESS varchar(255),
    VALUE varchar(255)
);

CREATE TABLE IF NOT EXISTS tokens (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    NAME varchar,
    DESCRIPTION varchar,
    CONTRACT_ADDRESS varchar(255),
    COLLECTORS_NOTE varchar,
    MEDIA jsonb,
    CHAIN varchar,
    OWNER_ADDRESS varchar(255),
    TOKEN_URI varchar,
    TOKEN_TYPE varchar,
    TOKEN_ID varchar,
    QUANTITY varchar,
    OWNERSHIP_HISTORY jsonb [],
    TOKEN_METADATA jsonb,
    EXTERNAL_URL varchar,
    BLOCK_NUMBER bigint
);

CREATE UNIQUE INDEX IF NOT EXISTS token_id_contract_address_owner_address_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS, OWNER_ADDRESS);

CREATE INDEX IF NOT EXISTS token_id_contract_address_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS);

CREATE INDEX IF NOT EXISTS owner_address_idx ON tokens (OWNER_ADDRESS);

CREATE INDEX IF NOT EXISTS contract_address_idx ON tokens (CONTRACT_ADDRESS);

CREATE INDEX IF NOT EXISTS block_number_idx ON tokens (BLOCK_NUMBER);

CREATE TABLE IF NOT EXISTS contracts (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    NAME varchar,
    SYMBOL varchar,
    ADDRESS varchar(255),
    CREATOR_ADDRESS varchar(255),
    LATEST_BLOCK bigint
);

CREATE UNIQUE INDEX IF NOT EXISTS address_idx ON contracts (ADDRESS);

CREATE TABLE IF NOT EXISTS login_attempts (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ADDRESS varchar(255),
    REQUEST_HOST_ADDRESS varchar(255),
    USER_EXISTS boolean,
    SIGNATURE varchar(255),
    SIGNATURE_VALID boolean,
    REQUEST_HEADERS jsonb,
    NONCE_VALUE varchar
);

CREATE TABLE IF NOT EXISTS features (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    REQUIRED_TOKEN varchar,
    REQUIRED_AMOUNT bigint,
    TOKEN_TYPE varchar,
    NAME varchar,
    IS_ENABLED boolean,
    ADMIN_ONLY boolean,
    FORCE_ENABLED_USER_IDS varchar(255) []
);

CREATE UNIQUE INDEX IF NOT EXISTS feature_name_idx ON features (NAME);

CREATE UNIQUE INDEX IF NOT EXISTS feature_required_token_idx ON features (REQUIRED_TOKEN);

CREATE TABLE IF NOT EXISTS backups (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    GALLERY_ID varchar(255),
    GALLERY jsonb
);

CREATE TABLE IF NOT EXISTS membership (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    TOKEN_ID varchar,
    NAME varchar,
    ASSET_URL varchar,
    OWNERS jsonb []
);

CREATE UNIQUE INDEX IF NOT EXISTS token_id_idx ON membership (TOKEN_ID);

CREATE TABLE IF NOT EXISTS access (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    USER_ID varchar(255),
    MOST_RECENT_BLOCK bigint,
    REQUIRED_TOKENS_OWNED jsonb,
    IS_ADMIN boolean
);

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

CREATE TABLE IF NOT EXISTS follows (
    ID varchar(255) PRIMARY KEY,
    FOLLOWER varchar(255) NOT NULL REFERENCES users (ID),
    FOLLOWEE varchar(255) NOT NULL REFERENCES users (ID),
    DELETED bool NOT NULL DEFAULT false,
    UNIQUE (FOLLOWER, FOLLOWEE)
);

CREATE TABLE IF NOT EXISTS early_access (
    address varchar(255) NOT NULL PRIMARY KEY
);

CREATE INDEX IF NOT EXISTS follows_follower_idx ON follows (FOLLOWER);

CREATE INDEX IF NOT EXISTS follows_followee_idx ON follows (FOLLOWEE);

CREATE INDEX IF NOT EXISTS user_id_collection_id_event_code_created_at ON collection_events (USER_ID, COLLECTION_ID, EVENT_CODE, CREATED_AT);

CREATE INDEX IF NOT EXISTS nfts_owner_address_idx ON nfts(owner_address);

CREATE INDEX IF NOT EXISTS user_id_event_code_last_updated ON user_events (USER_ID, EVENT_CODE, LAST_UPDATED DESC);

CREATE INDEX IF NOT EXISTS user_id_nft_id_event_code_last_updated ON nft_events (USER_ID, NFT_ID, EVENT_CODE, LAST_UPDATED DESC);

CREATE INDEX IF NOT EXISTS user_id_collection_id_event_code_last_updated ON collection_events (USER_ID, COLLECTION_ID, EVENT_CODE, LAST_UPDATED DESC);
