CREATE TABLE users (
    ID char(32) PRIMARY KEY,
    DELETED boolean,
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
    ID char(32) PRIMARY KEY,
    DELETED boolean,
    OWNER_USER_ID char(32),
    COLLECTIONS char(32) [],
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
    ID char(32) PRIMARY KEY,
    DELETED boolean,
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

CREATE TABLE collections (
    ID char(32) PRIMARY KEY,
    DELETED boolean,
    OWNER_USER_ID char(32),
    NFTS char(32) [],
    VERSION int,
    HIDDEN boolean,
    COLLECTORS_NOTE varchar,
    NAME varchar(255)
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
    ID char(32) PRIMARY KEY,
    DELETED boolean,
    VERSION int,
    USER_ID char(32),
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

CREATE
OR REPLACE FUNCTION ARRAY_DIFF(array1 anyarray, array2 anyarray) RETURNS anyarray LANGUAGE SQL IMMUTABLE AS $ $
SELECT
    COALESCE(array_agg(elem), '{}')
from
    UNNEST(array1) elem
where
    elem <> ALL(array2) $ $;