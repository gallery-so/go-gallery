-- name: FirstContract :one
-- sqlc needs at least one query in order to generate the models.
SELECT * FROM contracts LIMIT 1;

-- name: InsertStatistic :one
insert into blockchain_statistics (block_start, block_end) values ($1, $2) returning id;

-- name: UpdateStatisticTotalLogs :exec
update blockchain_statistics set total_logs = $1 where id = $2;

-- name: UpdateStatisticTotalTransfers :exec
update blockchain_statistics set total_transfers = $1 where id = $2;

-- name: UpdateStatisticTotalTokensAndContracts :exec
update blockchain_statistics set total_tokens = $1, total_contracts = $2 where id = $3;

-- name: UpdateStatisticSuccess :exec
update blockchain_statistics set success = $1, processing_time_seconds = $2 where id = $3;

-- name: UpdateStatisticContractStats :exec
update blockchain_statistics set contract_stats = $1 where id = $2;

-- name: UpdateStatisticTokenStats :exec
update blockchain_statistics set token_stats = $1 where id = $2;