CREATE TABLE users (
    ID varchar(32) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    USERNAME varchar(255),
    USERNAME_IDEMPOTENT varchar(255),
    ADDRESSES varchar(255) [],
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX users_username_idempotent ON users (USERNAME_IDEMPOTENT);

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