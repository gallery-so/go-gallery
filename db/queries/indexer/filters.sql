-- name: GetAddressFilterBatch :batchone
SELECT * FROM address_filters WHERE from_block = $1 AND to_block = $2 AND deleted = false;

-- name: AddAddressFilter :exec
INSERT INTO address_filters (id, from_block, to_block, bloom_filter, created_at, last_updated) VALUES ($1, $2, $3, $4, now(), now())
ON CONFLICT(from_block, to_block) DO UPDATE SET bloom_filter = EXCLUDED.bloom_filter, last_updated = now(), deleted = false;

-- name: BulkUpsertAddressFilters :exec
INSERT INTO address_filters (id, from_block, to_block, bloom_filter, created_at, last_updated) VALUES (unnest(@id::varchar[]), unnest(@from_block::bigint[]), unnest(@to_block::bigint[]), unnest(@bloom_filter::bytea[]), now(), now())
ON CONFLICT(from_block, to_block) DO UPDATE SET bloom_filter = EXCLUDED.bloom_filter, last_updated = now(), deleted = false;
