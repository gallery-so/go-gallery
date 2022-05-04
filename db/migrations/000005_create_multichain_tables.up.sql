CREATE TABLE IF NOT EXISTS addresses (
    ID varchar(255) PRIMARY KEY,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    ADDRESS_VALUE varchar(255),
    CHAIN int
);

CREATE TABLE IF NOT EXISTS wallets (
    ID varchar(255) PRIMARY KEY,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    ADDRESS varchar(255),
    WALLET_TYPE int
);


CREATE TABLE IF NOT EXISTS tokens (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    NAME varchar,
    DESCRIPTION varchar,
    CHAIN int,
    CONTRACT_ADDRESS varchar(255),
    COLLECTORS_NOTE varchar,
    MEDIA jsonb,
    OWNER_USER_ID varchar(255),
    OWNER_ADDRESSES varchar(255) [],
    TOKEN_URI varchar,
    TOKEN_TYPE varchar,
    TOKEN_ID varchar,
    QUANTITY varchar,
    OWNERSHIP_HISTORY jsonb [],
    TOKEN_METADATA jsonb,
    EXTERNAL_URL varchar,
    BLOCK_NUMBER bigint
);

CREATE TABLE IF NOT EXISTS contracts (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHAIN int,
    NAME varchar,
    SYMBOL varchar,
    ADDRESS varchar(255),
    CREATOR_ADDRESS varchar(255)
);

