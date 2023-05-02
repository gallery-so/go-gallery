// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.17.2
// source: token_gallery.sql

package coredb

import (
	"context"
	"time"

	"github.com/jackc/pgtype"
)

const upsertTokens = `-- name: UpsertTokens :many
with tids as (
  select unnest($27::varchar[]) as token_id, unnest($28::varchar[]) as contract, unnest($29::int[]) as chain
)
, limited_tms as (
  select
    tids.token_id,
    tids.contract,
    tids.chain,
    token_medias.id as media_id,
    ROW_NUMBER() OVER (PARTITION BY tids.token_id, tids.contract, tids.chain ORDER BY token_medias.last_updated) AS row_num
  from
    tids
    left join token_medias on (
      token_medias.token_id = tids.token_id
      and token_medias.contract_id = tids.contract
      and token_medias.chain = tids.chain
      and token_medias.active = true
      and token_medias.deleted = false
    )
)
, tms as (
  select
    array_agg(media_id) as media_id
  from
    limited_tms
  where
    row_num = 1
)
insert into tokens
(
  id
  , deleted
  , version
  , created_at
  , last_updated
  , name
  , description
  , collectors_note
  , token_type
  , token_id
  , quantity
  , ownership_history
  , media
  , fallback_media
  , token_metadata
  , external_url
  , block_number
  , owner_user_id
  , owned_by_wallets
  , chain
  , contract
  , is_user_marked_spam
  , is_provider_marked_spam
  , last_synced
  , token_uri
  , token_media_id
) (
  select
    id
    , deleted 
    , version
    , created_at
    , last_updated
    , name
    , description
    , collectors_note
    , token_type
    , token_id
    , quantity
    , ownership_history[ownership_history_start_idx::int:ownership_history_end_idx::int]
    , media
    , fallback_media
    , token_metadata
    , external_url
    , block_number
    , owner_user_id
    , owned_by_wallets[owned_by_wallets_start_idx::int:owned_by_wallets_end_idx::int]
    , chain
    , contract
    , is_user_marked_spam
    , is_provider_marked_spam
    , last_synced
    , token_uri
    , media_id
  from (
    select
      unnest($1::varchar[]) as id
      , unnest($2::boolean[]) as deleted
      , unnest($3::int[]) as version
      , unnest($4::timestamptz[]) as created_at
      , unnest($5::timestamptz[]) as last_updated
      , unnest($6::varchar[]) as name
      , unnest($7::varchar[]) as description
      , unnest($8::varchar[]) as collectors_note
      , unnest($9::varchar[]) as token_type
      , unnest($10::varchar[]) as quantity
      , $11::jsonb[] as ownership_history
      , unnest($12::int[]) as ownership_history_start_idx
      , unnest($13::int[]) as ownership_history_end_idx
      , unnest($14::jsonb[]) as media
      , unnest($15::jsonb[]) as fallback_media
      , unnest($16::jsonb[]) as token_metadata
      , unnest($17::varchar[]) as external_url
      , unnest($18::bigint[]) as block_number
      , unnest($19::varchar[]) as owner_user_id
      , $20::varchar[] as owned_by_wallets
      , unnest($21::int[]) as owned_by_wallets_start_idx
      , unnest($22::int[]) as owned_by_wallets_end_idx
      , unnest($23::bool[]) as is_user_marked_spam
      , unnest($24::bool[]) as is_provider_marked_spam
      , unnest($25::timestamptz[]) as last_synced
      , unnest($26::varchar[]) as token_uri
      , unnest($27::varchar[]) as token_id
      , unnest($28::varchar[]) as contract
      , unnest($29::int[]) as chain
      , unnest(tms.media_id) as media_id 
      from tms
  ) bulk_upsert
)
on conflict (token_id, contract, chain, owner_user_id) where deleted = false
do update set
  token_type = excluded.token_type
  , name = excluded.name
  , description = excluded.description
  , token_uri = excluded.token_uri
  , quantity = excluded.quantity
  , owned_by_wallets = excluded.owned_by_wallets
  , ownership_history = tokens.ownership_history || excluded.ownership_history
  , fallback_media = excluded.fallback_media
  , token_metadata = excluded.token_metadata
  , external_url = excluded.external_url
  , block_number = excluded.block_number
  , version = excluded.version
  , last_updated = excluded.last_updated
  , is_user_marked_spam = tokens.is_user_marked_spam
  , is_provider_marked_spam = excluded.is_provider_marked_spam
  , last_synced = greatest(excluded.last_synced,tokens.last_synced)
returning id, deleted, version, created_at, last_updated, name, description, collectors_note, media, token_uri, token_type, token_id, quantity, ownership_history, token_metadata, external_url, block_number, owner_user_id, owned_by_wallets, chain, contract, is_user_marked_spam, is_provider_marked_spam, last_synced, fallback_media, token_media_id
`

type UpsertTokensParams struct {
	ID                       []string
	Deleted                  []bool
	Version                  []int32
	CreatedAt                []time.Time
	LastUpdated              []time.Time
	Name                     []string
	Description              []string
	CollectorsNote           []string
	TokenType                []string
	Quantity                 []string
	OwnershipHistory         []pgtype.JSONB
	OwnershipHistoryStartIdx []int32
	OwnershipHistoryEndIdx   []int32
	Media                    []pgtype.JSONB
	FallbackMedia            []pgtype.JSONB
	TokenMetadata            []pgtype.JSONB
	ExternalUrl              []string
	BlockNumber              []int64
	OwnerUserID              []string
	OwnedByWallets           []string
	OwnedByWalletsStartIdx   []int32
	OwnedByWalletsEndIdx     []int32
	IsUserMarkedSpam         []bool
	IsProviderMarkedSpam     []bool
	LastSynced               []time.Time
	TokenUri                 []string
	TokenID                  []string
	Contract                 []string
	Chain                    []int32
}

func (q *Queries) UpsertTokens(ctx context.Context, arg UpsertTokensParams) ([]Token, error) {
	rows, err := q.db.Query(ctx, upsertTokens,
		arg.ID,
		arg.Deleted,
		arg.Version,
		arg.CreatedAt,
		arg.LastUpdated,
		arg.Name,
		arg.Description,
		arg.CollectorsNote,
		arg.TokenType,
		arg.Quantity,
		arg.OwnershipHistory,
		arg.OwnershipHistoryStartIdx,
		arg.OwnershipHistoryEndIdx,
		arg.Media,
		arg.FallbackMedia,
		arg.TokenMetadata,
		arg.ExternalUrl,
		arg.BlockNumber,
		arg.OwnerUserID,
		arg.OwnedByWallets,
		arg.OwnedByWalletsStartIdx,
		arg.OwnedByWalletsEndIdx,
		arg.IsUserMarkedSpam,
		arg.IsProviderMarkedSpam,
		arg.LastSynced,
		arg.TokenUri,
		arg.TokenID,
		arg.Contract,
		arg.Chain,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Token
	for rows.Next() {
		var i Token
		if err := rows.Scan(
			&i.ID,
			&i.Deleted,
			&i.Version,
			&i.CreatedAt,
			&i.LastUpdated,
			&i.Name,
			&i.Description,
			&i.CollectorsNote,
			&i.Media,
			&i.TokenUri,
			&i.TokenType,
			&i.TokenID,
			&i.Quantity,
			&i.OwnershipHistory,
			&i.TokenMetadata,
			&i.ExternalUrl,
			&i.BlockNumber,
			&i.OwnerUserID,
			&i.OwnedByWallets,
			&i.Chain,
			&i.Contract,
			&i.IsUserMarkedSpam,
			&i.IsProviderMarkedSpam,
			&i.LastSynced,
			&i.FallbackMedia,
			&i.TokenMediaID,
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
