// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.18.0
// source: query.sql

package indexerdb

import (
	"context"
	"database/sql"

	"github.com/jackc/pgtype"
	"github.com/mikeydub/go-gallery/service/persist"
)

const firstContract = `-- name: FirstContract :one
SELECT id, deleted, version, created_at, last_updated, name, symbol, address, creator_address, chain, latest_block, owner_address, owner_method FROM contracts LIMIT 1
`

// sqlc needs at least one query in order to generate the models.
func (q *Queries) FirstContract(ctx context.Context) (Contract, error) {
	row := q.db.QueryRow(ctx, firstContract)
	var i Contract
	err := row.Scan(
		&i.ID,
		&i.Deleted,
		&i.Version,
		&i.CreatedAt,
		&i.LastUpdated,
		&i.Name,
		&i.Symbol,
		&i.Address,
		&i.CreatorAddress,
		&i.Chain,
		&i.LatestBlock,
		&i.OwnerAddress,
		&i.OwnerMethod,
	)
	return i, err
}

const getContractsByIDRange = `-- name: GetContractsByIDRange :many
SELECT
    contracts.id, contracts.deleted, contracts.version, contracts.created_at, contracts.last_updated, contracts.name, contracts.symbol, contracts.address, contracts.creator_address, contracts.chain, contracts.latest_block, contracts.owner_address, contracts.owner_method
FROM contracts
WHERE contracts.deleted = false
AND (contracts.owner_address IS NULL OR contracts.owner_address = '' OR contracts.creator_address IS NULL OR contracts.creator_address = '') 
AND contracts.id > $1 AND contracts.id < $2
ORDER BY contracts.id
`

type GetContractsByIDRangeParams struct {
	StartID string
	EndID   string
}

func (q *Queries) GetContractsByIDRange(ctx context.Context, arg GetContractsByIDRangeParams) ([]Contract, error) {
	rows, err := q.db.Query(ctx, getContractsByIDRange, arg.StartID, arg.EndID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Contract
	for rows.Next() {
		var i Contract
		if err := rows.Scan(
			&i.ID,
			&i.Deleted,
			&i.Version,
			&i.CreatedAt,
			&i.LastUpdated,
			&i.Name,
			&i.Symbol,
			&i.Address,
			&i.CreatorAddress,
			&i.Chain,
			&i.LatestBlock,
			&i.OwnerAddress,
			&i.OwnerMethod,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getReprocessJobRangeByID = `-- name: GetReprocessJobRangeByID :one
select id, start_id, end_id from reprocess_jobs where id = $1
`

func (q *Queries) GetReprocessJobRangeByID(ctx context.Context, id int) (ReprocessJob, error) {
	row := q.db.QueryRow(ctx, getReprocessJobRangeByID, id)
	var i ReprocessJob
	err := row.Scan(&i.ID, &i.StartID, &i.EndID)
	return i, err
}

const insertStatistic = `-- name: InsertStatistic :one
insert into blockchain_statistics (block_start, block_end) values ($1, $2) returning id
`

type InsertStatisticParams struct {
	BlockStart persist.BlockNumber
	BlockEnd   persist.BlockNumber
}

func (q *Queries) InsertStatistic(ctx context.Context, arg InsertStatisticParams) (persist.DBID, error) {
	row := q.db.QueryRow(ctx, insertStatistic, arg.BlockStart, arg.BlockEnd)
	var id persist.DBID
	err := row.Scan(&id)
	return id, err
}

const updateContractOwnerByID = `-- name: UpdateContractOwnerByID :exec
update contracts set owner_address = $1, creator_address = $2, owner_method = $3 where id = $4 and deleted = false
`

type UpdateContractOwnerByIDParams struct {
	OwnerAddress   persist.EthereumAddress
	CreatorAddress persist.EthereumAddress
	OwnerMethod    persist.ContractOwnerMethod
	ID             persist.DBID
}

func (q *Queries) UpdateContractOwnerByID(ctx context.Context, arg UpdateContractOwnerByIDParams) error {
	_, err := q.db.Exec(ctx, updateContractOwnerByID,
		arg.OwnerAddress,
		arg.CreatorAddress,
		arg.OwnerMethod,
		arg.ID,
	)
	return err
}

const updateStatisticContractStats = `-- name: UpdateStatisticContractStats :exec
update blockchain_statistics set contract_stats = $1 where id = $2
`

type UpdateStatisticContractStatsParams struct {
	ContractStats pgtype.JSONB
	ID            persist.DBID
}

func (q *Queries) UpdateStatisticContractStats(ctx context.Context, arg UpdateStatisticContractStatsParams) error {
	_, err := q.db.Exec(ctx, updateStatisticContractStats, arg.ContractStats, arg.ID)
	return err
}

const updateStatisticSuccess = `-- name: UpdateStatisticSuccess :exec
update blockchain_statistics set success = $1, processing_time_seconds = $2 where id = $3
`

type UpdateStatisticSuccessParams struct {
	Success               bool
	ProcessingTimeSeconds sql.NullInt64
	ID                    persist.DBID
}

func (q *Queries) UpdateStatisticSuccess(ctx context.Context, arg UpdateStatisticSuccessParams) error {
	_, err := q.db.Exec(ctx, updateStatisticSuccess, arg.Success, arg.ProcessingTimeSeconds, arg.ID)
	return err
}

const updateStatisticTokenStats = `-- name: UpdateStatisticTokenStats :exec
update blockchain_statistics set token_stats = $1 where id = $2
`

type UpdateStatisticTokenStatsParams struct {
	TokenStats pgtype.JSONB
	ID         persist.DBID
}

func (q *Queries) UpdateStatisticTokenStats(ctx context.Context, arg UpdateStatisticTokenStatsParams) error {
	_, err := q.db.Exec(ctx, updateStatisticTokenStats, arg.TokenStats, arg.ID)
	return err
}

const updateStatisticTotalLogs = `-- name: UpdateStatisticTotalLogs :exec
update blockchain_statistics set total_logs = $1 where id = $2
`

type UpdateStatisticTotalLogsParams struct {
	TotalLogs sql.NullInt64
	ID        persist.DBID
}

func (q *Queries) UpdateStatisticTotalLogs(ctx context.Context, arg UpdateStatisticTotalLogsParams) error {
	_, err := q.db.Exec(ctx, updateStatisticTotalLogs, arg.TotalLogs, arg.ID)
	return err
}

const updateStatisticTotalTokensAndContracts = `-- name: UpdateStatisticTotalTokensAndContracts :exec
update blockchain_statistics set total_tokens = $1, total_contracts = $2 where id = $3
`

type UpdateStatisticTotalTokensAndContractsParams struct {
	TotalTokens    sql.NullInt64
	TotalContracts sql.NullInt64
	ID             persist.DBID
}

func (q *Queries) UpdateStatisticTotalTokensAndContracts(ctx context.Context, arg UpdateStatisticTotalTokensAndContractsParams) error {
	_, err := q.db.Exec(ctx, updateStatisticTotalTokensAndContracts, arg.TotalTokens, arg.TotalContracts, arg.ID)
	return err
}

const updateStatisticTotalTransfers = `-- name: UpdateStatisticTotalTransfers :exec
update blockchain_statistics set total_transfers = $1 where id = $2
`

type UpdateStatisticTotalTransfersParams struct {
	TotalTransfers sql.NullInt64
	ID             persist.DBID
}

func (q *Queries) UpdateStatisticTotalTransfers(ctx context.Context, arg UpdateStatisticTotalTransfersParams) error {
	_, err := q.db.Exec(ctx, updateStatisticTotalTransfers, arg.TotalTransfers, arg.ID)
	return err
}
