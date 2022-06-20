
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

CREATE UNIQUE INDEX IF NOT EXISTS opensea_id_owner_address_inx ON nfts (OPENSEA_ID, OWNER_ADDRESS);
