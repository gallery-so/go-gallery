CREATE TABLE users (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    USERNAME varchar(255),
    USERNAME_IDEMPOTENT varchar(255),
    ADDRESSES varchar(255) []
);

CREATE UNIQUE INDEX users_username_idempotent ON users (USERNAME_IDEMPOTENT);

CREATE TABLE galleries (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    VERSION int,
    OWNER_USER_ID varchar(255),
    COLLECTIONS varchar(255) []
);

CREATE TABLE nfts (
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
    COLLECTORS_NOTE varchar,
    TOKEN_COLLECTION_NAME varchar,
    TOKEN_METADATA_URL varchar
);

CREATE UNIQUE INDEX opensea_id_owner_address_inx ON nfts (OPENSEA_ID, OWNER_ADDRESS);

CREATE TABLE collections (
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

CREATE TABLE nonces (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    USER_ID varchar(255),
    ADDRESS varchar(255),
    VALUE varchar(255)
);

CREATE TABLE tokens (
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

CREATE UNIQUE INDEX token_id_contract_address_owner_address_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS, OWNER_ADDRESS);

CREATE INDEX block_number_idx ON tokens (BLOCK_NUMBER);

CREATE TABLE contracts (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    NAME varchar,
    SYMBOL varchar,
    ADDRESS varchar(255),
    LATEST_BLOCK bigint
);

CREATE UNIQUE INDEX address_idx ON contracts (ADDRESS);

CREATE TABLE login_attempts (
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

CREATE TABLE features (
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

CREATE UNIQUE INDEX feature_name_idx ON features (NAME);

CREATE UNIQUE INDEX feature_required_token_idx ON features (REQUIRED_TOKEN);

CREATE TABLE backups (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    GALLERY_ID varchar(255),
    GALLERY jsonb
);

CREATE TABLE membership (
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

CREATE UNIQUE INDEX token_id_idx ON membership (TOKEN_ID);

CREATE TABLE access (
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