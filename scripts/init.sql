CREATE TABLE users (
    ID varchar(32) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    VERSION int,
    USERNAME varchar(255),
    USERNAME_IDEMPOTENT varchar(255),
    ADDRESSES varchar(255) []
);

COPY users
FROM
    '/import/users.csv' with (FORMAT csv);

ALTER TABLE
    users
ADD
    COLUMN LAST_UPDATED timestamp DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE
    users
ADD
    COLUMN CREATED_AT timestamp DEFAULT CURRENT_TIMESTAMP;

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
    COLUMN LAST_UPDATED timestamp DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE
    galleries
ADD
    COLUMN CREATED_AT timestamp DEFAULT CURRENT_TIMESTAMP;

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
    COLUMN LAST_UPDATED timestamp DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE
    nfts
ADD
    COLUMN CREATED_AT timestamp DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE
    nfts
ADD
    COLUMN TOKEN_COLLECTION_NAME varchar;

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
    COLUMN LAST_UPDATED timestamp DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE
    collections
ADD
    COLUMN CREATED_AT timestamp DEFAULT CURRENT_TIMESTAMP;

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
    COLUMN LAST_UPDATED timestamp DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE
    nonces
ADD
    COLUMN CREATED_AT timestamp DEFAULT CURRENT_TIMESTAMP;