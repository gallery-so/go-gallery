CREATE TABLE users (
    ID varchar(32) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    USERNAME varchar(255),
    USERNAME_IDEMPOTENT varchar(255),
    ADDRESSES varchar(255) []
);

CREATE UNIQUE INDEX users_username_idempotent ON users (USERNAME_IDEMPOTENT);

COPY users
FROM
    '/import/users.csv' with (FORMAT csv);

ALTER TABLE
    users
ADD
    COLUMN LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
ADD
    COLUMN CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE TABLE galleries (
    ID varchar(32) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    OWNER_USER_ID varchar(32),
    COLLECTIONS varchar(32) [],
    VERSION int
);

COPY galleries
FROM
    '/import/galleries.csv' with (FORMAT csv);

ALTER TABLE
    galleries
ADD
    COLUMN LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
ADD
    COLUMN CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE TABLE nfts (
    ID varchar(32) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    NAME varchar,
    DESCRIPTION varchar,
    EXTERNAL_URL varchar,
    CREATOR_ADDRESS varchar(255),
    CREATOR_NAME varchar,
    OWNER_ADDRESS varchar(255),
    MULTIPLE_OWNERS boolean,
    CONTRACT json,
    OPENSEA_ID bigint,
    OPENSEA_TOKEN_ID varchar(255),
    IMAGE_URL varchar,
    IMAGE_THUMBNAIL_URL varchar,
    IMAGE_PREVIEW_URL varchar,
    IMAGE_ORIGINAL_URL varchar,
    ANIMATION_URL varchar,
    ANIMATION_ORIGINAL_URL varchar
);

COPY nfts
FROM
    '/import/nfts.csv' with (FORMAT csv);

ALTER TABLE
    nfts
ADD
    COLUMN LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
ADD
    COLUMN CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
ADD
    COLUMN TOKEN_COLLECTION_NAME varchar,
ADD
    COLUMN COLLECTORS_NOTE varchar;

CREATE UNIQUE INDEX opensea_id_owner_address_inx ON nfts (OPENSEA_ID, OWNER_ADDRESS);

CREATE TABLE collections (
    ID varchar(32) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    OWNER_USER_ID varchar(32),
    NFTS varchar(32) [],
    VERSION int,
    HIDDEN boolean NOT NULL DEFAULT false,
    COLLECTORS_NOTE varchar,
    NAME varchar(255),
    LAYOUT json
);

COPY collections
FROM
    '/import/collections.csv' with (FORMAT csv);

ALTER TABLE
    collections
ADD
    COLUMN LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
ADD
    COLUMN CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE TABLE nonces (
    ID varchar(32) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    USER_ID varchar(32),
    ADDRESS varchar(255),
    VALUE varchar(255)
);

COPY nonces
FROM
    '/import/nonces.csv' with (FORMAT csv);

ALTER TABLE
    nonces
ADD
    COLUMN LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
ADD
    COLUMN CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE TABLE tokens (
    ID varchar(32) PRIMARY KEY,
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

CREATE UNIQUE INDEX token_id_contract_address_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS)
WHERE
    TOKEN_TYPE = 'ERC-721';

CREATE UNIQUE INDEX token_id_contract_address_owner_address_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS, OWNER_ADDRESS)
WHERE
    TOKEN_TYPE = 'ERC-1155';

CREATE INDEX block_number_idx ON tokens (BLOCK_NUMBER);

CREATE TABLE contracts (
    ID varchar(32) PRIMARY KEY,
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
    ID varchar(32) PRIMARY KEY,
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
    ID varchar(32) PRIMARY KEY,
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
    FORCE_ENABLED_USER_IDS varchar(32) []
);

CREATE UNIQUE INDEX feature_name_idx ON features (NAME);

CREATE UNIQUE INDEX feature_required_token_idx ON features (REQUIRED_TOKEN);

CREATE TABLE backups (
    ID varchar(32) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    GALLERY_ID varchar(32),
    GALLERY jsonb
);

CREATE TABLE membership (
    ID varchar(32) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    TOKEN_ID varchar,
    NAME varchar,
    ASSET_URL varchar,
    OWNERS jsonb []
);

CREATE TABLE access (
    ID varchar(32) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    USER_ID varchar(32),
    MOST_RECENT_BLOCK bigint,
    REQUIRED_TOKENS_OWNED jsonb,
    IS_ADMIN boolean
);