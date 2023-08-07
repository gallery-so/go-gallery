// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.18.0
// source: token_gallery.sql

package coredb

import (
	"context"

	"github.com/jackc/pgtype"
)

const upsertTokens = `-- name: UpsertTokens :many
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
  , fallback_media
  , external_url
  , block_number
  , owner_user_id
  , owned_by_wallets
  , is_creator_token
  , chain
  , contract
  , is_provider_marked_spam
  , last_synced
  , token_uri
  , token_media_id
) (
  select
    id
    , false
    , version
    , now()
    , now()
    , name
    , description
    , collectors_note
    , token_type
    , token_id
    , quantity
    , case when $1::bool then ownership_history[ownership_history_start_idx::int:ownership_history_end_idx::int] else '{}' end
    , fallback_media
    , external_url
    , block_number
    , owner_user_id
    , case when $1 then owned_by_wallets[owned_by_wallets_start_idx::int:owned_by_wallets_end_idx::int] else '{}' end
    , case when $2::bool then is_creator_token else false end
    , chain
    , contract
    , is_provider_marked_spam
    , now()
    , token_uri
    , (select tm.id
       from token_medias tm
       where tm.token_id = bulk_upsert.token_id
         and tm.contract_id = bulk_upsert.contract
         and tm.chain = bulk_upsert.chain
         and tm.active = true
         and tm.deleted = false
        limit 1
      ) as token_media_id
  from (
    select unnest($3::varchar[]) as id
      , unnest($4::int[]) as version
      , unnest($5::varchar[]) as name
      , unnest($6::varchar[]) as description
      , unnest($7::varchar[]) as collectors_note
      , unnest($8::varchar[]) as token_type
      , unnest($9::varchar[]) as quantity
      , $10::jsonb[] as ownership_history
      , unnest($11::int[]) as ownership_history_start_idx
      , unnest($12::int[]) as ownership_history_end_idx
      , unnest($13::jsonb[]) as fallback_media
      , unnest($14::varchar[]) as external_url
      , unnest($15::bigint[]) as block_number
      , unnest($16::varchar[]) as owner_user_id
      , $17::varchar[] as owned_by_wallets
      , unnest($18::int[]) as owned_by_wallets_start_idx
      , unnest($19::int[]) as owned_by_wallets_end_idx
      , unnest($20::bool[]) as is_creator_token
      , unnest($21::bool[]) as is_provider_marked_spam
      , unnest($22::varchar[]) as token_uri
      , unnest($23::varchar[]) as token_id
      , unnest($24::varchar[]) as contract
      , unnest($25::int[]) as chain
  ) bulk_upsert
)
on conflict (token_id, contract, chain, owner_user_id) where deleted = false
do update set
  token_type = excluded.token_type
  , name = excluded.name
  , description = excluded.description
  , token_uri = excluded.token_uri
  , quantity = excluded.quantity
  , owned_by_wallets = case when $1 then excluded.owned_by_wallets else tokens.owned_by_wallets end
  , ownership_history = case when $1 then tokens.ownership_history || excluded.ownership_history else tokens.ownership_history end
  , is_creator_token = case when $2 then excluded.is_creator_token else tokens.is_creator_token end
  , fallback_media = excluded.fallback_media
  , external_url = excluded.external_url
  , block_number = excluded.block_number
  , version = excluded.version
  , last_updated = excluded.last_updated
  , is_provider_marked_spam = excluded.is_provider_marked_spam
  , last_synced = greatest(excluded.last_synced,tokens.last_synced)
returning id, deleted, version, created_at, last_updated, name, description, collectors_note, token_uri, token_type, token_id, quantity, ownership_history, external_url, block_number, owner_user_id, owned_by_wallets, chain, contract, is_user_marked_spam, is_provider_marked_spam, last_synced, fallback_media, token_media_id, is_creator_token, is_holder_token, displayable
`

type UpsertTokensParams struct {
	SetHolderFields          bool           `json:"set_holder_fields"`
	SetCreatorFields         bool           `json:"set_creator_fields"`
	ID                       []string       `json:"id"`
	Version                  []int32        `json:"version"`
	Name                     []string       `json:"name"`
	Description              []string       `json:"description"`
	CollectorsNote           []string       `json:"collectors_note"`
	TokenType                []string       `json:"token_type"`
	Quantity                 []string       `json:"quantity"`
	OwnershipHistory         []pgtype.JSONB `json:"ownership_history"`
	OwnershipHistoryStartIdx []int32        `json:"ownership_history_start_idx"`
	OwnershipHistoryEndIdx   []int32        `json:"ownership_history_end_idx"`
	FallbackMedia            []pgtype.JSONB `json:"fallback_media"`
	ExternalUrl              []string       `json:"external_url"`
	BlockNumber              []int64        `json:"block_number"`
	OwnerUserID              []string       `json:"owner_user_id"`
	OwnedByWallets           []string       `json:"owned_by_wallets"`
	OwnedByWalletsStartIdx   []int32        `json:"owned_by_wallets_start_idx"`
	OwnedByWalletsEndIdx     []int32        `json:"owned_by_wallets_end_idx"`
	IsCreatorToken           []bool         `json:"is_creator_token"`
	IsProviderMarkedSpam     []bool         `json:"is_provider_marked_spam"`
	TokenUri                 []string       `json:"token_uri"`
	TokenID                  []string       `json:"token_id"`
	Contract                 []string       `json:"contract"`
	Chain                    []int32        `json:"chain"`
}

func (q *Queries) UpsertTokens(ctx context.Context, arg UpsertTokensParams) ([]Token, error) {
	rows, err := q.db.Query(ctx, upsertTokens,
		arg.SetHolderFields,
		arg.SetCreatorFields,
		arg.ID,
		arg.Version,
		arg.Name,
		arg.Description,
		arg.CollectorsNote,
		arg.TokenType,
		arg.Quantity,
		arg.OwnershipHistory,
		arg.OwnershipHistoryStartIdx,
		arg.OwnershipHistoryEndIdx,
		arg.FallbackMedia,
		arg.ExternalUrl,
		arg.BlockNumber,
		arg.OwnerUserID,
		arg.OwnedByWallets,
		arg.OwnedByWalletsStartIdx,
		arg.OwnedByWalletsEndIdx,
		arg.IsCreatorToken,
		arg.IsProviderMarkedSpam,
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
			&i.TokenUri,
			&i.TokenType,
			&i.TokenID,
			&i.Quantity,
			&i.OwnershipHistory,
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
			&i.IsCreatorToken,
			&i.IsHolderToken,
			&i.Displayable,
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
