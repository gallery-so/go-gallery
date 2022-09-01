-- name: GetBlockFilterBatch :batchone
SELECT * FROM block_filters WHERE from_block = $1 AND to_block = $2 AND deleted = false;

-- name: BulkUpsertBlockFilter :exec
INSERT INTO block_filters (id, from_block, to_block, bloom_filter, created_at, last_updated) values (unnest(@id::varchar[]), unnest(@from_block::bigint[]), unnest(@to_block::bigint[]), unnest(@bloom_filter::bytea[]), now(), now())
ON CONFLICT(from_block, to_block) DO UPDATE SET bloom_filter = EXCLUDED.bloom_filter, last_updated = now(), deleted = false;