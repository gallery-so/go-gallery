<<<<<<< HEAD:db/migrations/000026_add_block_bloom.up.sql
CREATE TABLE IF NOT EXISTS block_blooms (
=======
CREATE TABLE IF NOT EXISTS address_filters (
>>>>>>> 7aeaece (Rename table to address_filter):db/migrations/indexer/000002_add_block_filters.up.sql
    ID varchar(255) PRIMARY KEY,
    FROM_BLOCK bigint NOT NULL,
    TO_BLOCK bigint NOT NULL,
    BLOOM_FILTER bytea,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DELETED boolean NOT NULL DEFAULT false
);

<<<<<<< HEAD:db/migrations/000026_add_block_bloom.up.sql
CREATE UNIQUE INDEX block_blooms_from_block_to_block ON block_blooms (FROM_BLOCK, TO_BLOCK);
=======
CREATE UNIQUE INDEX address_filters_from_block_to_block ON address_filters (FROM_BLOCK, TO_BLOCK);
>>>>>>> 7aeaece (Rename table to address_filter):db/migrations/indexer/000002_add_block_filters.up.sql
