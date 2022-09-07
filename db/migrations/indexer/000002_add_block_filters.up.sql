CREATE TABLE IF NOT EXISTS address_filters (
    ID varchar(255) PRIMARY KEY,
    FROM_BLOCK bigint NOT NULL,
    TO_BLOCK bigint NOT NULL,
    BLOOM_FILTER bytea,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DELETED boolean NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX address_filters_from_block_to_block ON address_filters (FROM_BLOCK, TO_BLOCK);
