// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.18.0
// source: token_gallery.sql

package coredb

import (
	"context"
	"database/sql"
	"time"

	"github.com/jackc/pgtype"
)

const deleteTokensBeforeTimestamp = `-- name: DeleteTokensBeforeTimestamp :execrows
update tokens t
set owned_by_wallets = case when $1::bool then '{}' else owned_by_wallets end,
    is_creator_token = case when $2::bool then false else is_creator_token end,
    last_updated = now()
from token_definitions td
where
  -- Guard against only_from_user_id and only_from_contract_ids both being null/empty, as this would
  -- result in deleting more tokens than intended.
  ($3::text is not null or cardinality($4::text[]) > 0)
  and ($3 is null or t.owner_user_id = $3)
  and (cardinality($4) = 0 or td.contract_id = any($4))
  and (cardinality($5::int[]) = 0 or td.chain = any($5))
  and (($1 and t.is_holder_token) or ($2 and t.is_creator_token))
  and t.token_definition_id = td.id
  and t.deleted = false
  and td.deleted = false
  and t.last_synced < $6
`

type DeleteTokensBeforeTimestampParams struct {
	RemoveHolderStatus  bool           `db:"remove_holder_status" json:"remove_holder_status"`
	RemoveCreatorStatus bool           `db:"remove_creator_status" json:"remove_creator_status"`
	OnlyFromUserID      sql.NullString `db:"only_from_user_id" json:"only_from_user_id"`
	OnlyFromContractIds []string       `db:"only_from_contract_ids" json:"only_from_contract_ids"`
	OnlyFromChains      []int32        `db:"only_from_chains" json:"only_from_chains"`
	Timestamp           time.Time      `db:"timestamp" json:"timestamp"`
}

func (q *Queries) DeleteTokensBeforeTimestamp(ctx context.Context, arg DeleteTokensBeforeTimestampParams) (int64, error) {
	result, err := q.db.Exec(ctx, deleteTokensBeforeTimestamp,
		arg.RemoveHolderStatus,
		arg.RemoveCreatorStatus,
		arg.OnlyFromUserID,
		arg.OnlyFromContractIds,
		arg.OnlyFromChains,
		arg.Timestamp,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

const upsertTokens = `-- name: UpsertTokens :many
with token_definitions_insert as (
  insert into token_definitions
  ( 
    id
    , created_at
    , last_updated
    , deleted
    , name
    , description
    , token_type
    , token_id
    , external_url
    , chain
    , fallback_media
    , contract_address
    , contract_id
    , metadata
  ) (
    select unnest($1::varchar[]) as id
      , now()
      , now()
      , false
      , unnest($2::varchar[]) as name
      , unnest($3::varchar[]) as description
      , unnest($4::varchar[]) as token_type
      , unnest($5::varchar[]) as token_id
      , unnest($6::varchar[]) as external_url
      , unnest($7::int[]) as chain
      , unnest($8::jsonb[]) as fallback_media
      , unnest($9::varchar[]) as contract_address
      , unnest($10::varchar[]) as contract_id
      , unnest($11::jsonb[]) as metadata
  )
  on conflict (chain, contract_id, token_id) where deleted = false
  do update set
    last_updated = excluded.last_updated
    , name = coalesce(nullif(excluded.name, ''), nullif(token_definitions.name, ''))
    , description = coalesce(nullif(excluded.description, ''), nullif(token_definitions.description, ''))
    , token_type = excluded.token_type
    , external_url = coalesce(nullif(excluded.external_url, ''), nullif(token_definitions.external_url, ''))
    , fallback_media = excluded.fallback_media
    , contract_address = excluded.contract_address
    , metadata = excluded.metadata
  returning id, created_at, last_updated, deleted, name, description, token_type, token_id, external_url, chain, metadata, fallback_media, contract_address, contract_id, token_media_id
)
, tokens_insert as (
  insert into tokens
  (
    id
    , deleted
    , version
    , created_at
    , last_updated
    , collectors_note
    , quantity
    , block_number
    , owner_user_id
    , owned_by_wallets
    , is_creator_token
    , last_synced
    , token_definition_id
    , contract_id
  ) (
    select
      bulk_upsert.id
      , false
      , bulk_upsert.version
      , now()
      , now()
      , bulk_upsert.collectors_note
      , bulk_upsert.quantity
      , bulk_upsert.block_number
      , bulk_upsert.owner_user_id
      , case when $12::bool then bulk_upsert.owned_by_wallets[bulk_upsert.owned_by_wallets_start_idx::int:bulk_upsert.owned_by_wallets_end_idx::int] else '{}' end
      , case when $13::bool then bulk_upsert.is_creator_token else false end
      , now()
      , token_definitions_insert.id
      , bulk_upsert.contract_id
    from (
      select unnest($14::varchar[]) as id
        , unnest($15::int[]) as version
        , unnest($16::varchar[]) as collectors_note
        , unnest($17::varchar[]) as quantity
        , unnest($18::bigint[]) as block_number
        , unnest($19::varchar[]) as owner_user_id
        , $20::varchar[] as owned_by_wallets
        , unnest($21::int[]) as owned_by_wallets_start_idx
        , unnest($22::int[]) as owned_by_wallets_end_idx
        , unnest($23::bool[]) as is_creator_token
        , unnest($24::varchar[]) as token_id
        , unnest($25::varchar[]) as contract_address
        , unnest($26::int[]) as chain
        , unnest($27::varchar[]) as contract_id
    ) bulk_upsert
    join token_definitions_insert on (bulk_upsert.chain, bulk_upsert.contract_address, bulk_upsert.token_id) = (token_definitions_insert.chain, token_definitions_insert.contract_address, token_definitions_insert.token_id)
  )
  on conflict (owner_user_id, token_definition_id) where deleted = false
  do update set
    quantity = excluded.quantity
    , owned_by_wallets = case when $12 then excluded.owned_by_wallets else tokens.owned_by_wallets end
    , is_creator_token = case when $13 then excluded.is_creator_token else tokens.is_creator_token end
    , block_number = excluded.block_number
    , version = excluded.version
    , last_updated = excluded.last_updated
    , last_synced = greatest(excluded.last_synced,tokens.last_synced)
    , contract_id = excluded.contract_id
  returning id, deleted, version, created_at, last_updated, name__deprecated, description__deprecated, collectors_note, token_type__deprecated, token_id__deprecated, quantity, ownership_history__deprecated, external_url__deprecated, block_number, owner_user_id, owned_by_wallets, chain__deprecated, contract_id, is_user_marked_spam, is_provider_marked_spam__deprecated, last_synced, token_uri__deprecated, fallback_media__deprecated, token_media_id__deprecated, is_creator_token, token_definition_id, is_holder_token, displayable
)
select tokens.id, tokens.deleted, tokens.version, tokens.created_at, tokens.last_updated, tokens.name__deprecated, tokens.description__deprecated, tokens.collectors_note, tokens.token_type__deprecated, tokens.token_id__deprecated, tokens.quantity, tokens.ownership_history__deprecated, tokens.external_url__deprecated, tokens.block_number, tokens.owner_user_id, tokens.owned_by_wallets, tokens.chain__deprecated, tokens.contract_id, tokens.is_user_marked_spam, tokens.is_provider_marked_spam__deprecated, tokens.last_synced, tokens.token_uri__deprecated, tokens.fallback_media__deprecated, tokens.token_media_id__deprecated, tokens.is_creator_token, tokens.token_definition_id, tokens.is_holder_token, tokens.displayable, token_definitions.id, token_definitions.created_at, token_definitions.last_updated, token_definitions.deleted, token_definitions.name, token_definitions.description, token_definitions.token_type, token_definitions.token_id, token_definitions.external_url, token_definitions.chain, token_definitions.metadata, token_definitions.fallback_media, token_definitions.contract_address, token_definitions.contract_id, token_definitions.token_media_id, contracts.id, contracts.deleted, contracts.version, contracts.created_at, contracts.last_updated, contracts.name, contracts.symbol, contracts.address, contracts.creator_address, contracts.chain, contracts.profile_banner_url, contracts.profile_image_url, contracts.badge_url, contracts.description, contracts.owner_address, contracts.is_provider_marked_spam, contracts.parent_id, contracts.override_creator_user_id, contracts.l1_chain
from tokens_insert tokens
join token_definitions_insert token_definitions on tokens.token_definition_id = token_definitions.id
join contracts on token_definitions.contract_id = contracts.id
`

type UpsertTokensParams struct {
	DefinitionDbid              []string       `db:"definition_dbid" json:"definition_dbid"`
	DefinitionName              []string       `db:"definition_name" json:"definition_name"`
	DefinitionDescription       []string       `db:"definition_description" json:"definition_description"`
	DefinitionTokenType         []string       `db:"definition_token_type" json:"definition_token_type"`
	DefinitionTokenID           []string       `db:"definition_token_id" json:"definition_token_id"`
	DefinitionExternalUrl       []string       `db:"definition_external_url" json:"definition_external_url"`
	DefinitionChain             []int32        `db:"definition_chain" json:"definition_chain"`
	DefinitionFallbackMedia     []pgtype.JSONB `db:"definition_fallback_media" json:"definition_fallback_media"`
	DefinitionContractAddress   []string       `db:"definition_contract_address" json:"definition_contract_address"`
	DefinitionContractID        []string       `db:"definition_contract_id" json:"definition_contract_id"`
	DefinitionMetadata          []pgtype.JSONB `db:"definition_metadata" json:"definition_metadata"`
	SetHolderFields             bool           `db:"set_holder_fields" json:"set_holder_fields"`
	SetCreatorFields            bool           `db:"set_creator_fields" json:"set_creator_fields"`
	TokenDbid                   []string       `db:"token_dbid" json:"token_dbid"`
	TokenVersion                []int32        `db:"token_version" json:"token_version"`
	TokenCollectorsNote         []string       `db:"token_collectors_note" json:"token_collectors_note"`
	TokenQuantity               []string       `db:"token_quantity" json:"token_quantity"`
	TokenBlockNumber            []int64        `db:"token_block_number" json:"token_block_number"`
	TokenOwnerUserID            []string       `db:"token_owner_user_id" json:"token_owner_user_id"`
	TokenOwnedByWallets         []string       `db:"token_owned_by_wallets" json:"token_owned_by_wallets"`
	TokenOwnedByWalletsStartIdx []int32        `db:"token_owned_by_wallets_start_idx" json:"token_owned_by_wallets_start_idx"`
	TokenOwnedByWalletsEndIdx   []int32        `db:"token_owned_by_wallets_end_idx" json:"token_owned_by_wallets_end_idx"`
	TokenIsCreatorToken         []bool         `db:"token_is_creator_token" json:"token_is_creator_token"`
	TokenTokenID                []string       `db:"token_token_id" json:"token_token_id"`
	TokenContractAddress        []string       `db:"token_contract_address" json:"token_contract_address"`
	TokenChain                  []int32        `db:"token_chain" json:"token_chain"`
	TokenContractID             []string       `db:"token_contract_id" json:"token_contract_id"`
}

type UpsertTokensRow struct {
	Token           Token           `db:"token" json:"token"`
	TokenDefinition TokenDefinition `db:"tokendefinition" json:"tokendefinition"`
	Contract        Contract        `db:"contract" json:"contract"`
}

func (q *Queries) UpsertTokens(ctx context.Context, arg UpsertTokensParams) ([]UpsertTokensRow, error) {
	rows, err := q.db.Query(ctx, upsertTokens,
		arg.DefinitionDbid,
		arg.DefinitionName,
		arg.DefinitionDescription,
		arg.DefinitionTokenType,
		arg.DefinitionTokenID,
		arg.DefinitionExternalUrl,
		arg.DefinitionChain,
		arg.DefinitionFallbackMedia,
		arg.DefinitionContractAddress,
		arg.DefinitionContractID,
		arg.DefinitionMetadata,
		arg.SetHolderFields,
		arg.SetCreatorFields,
		arg.TokenDbid,
		arg.TokenVersion,
		arg.TokenCollectorsNote,
		arg.TokenQuantity,
		arg.TokenBlockNumber,
		arg.TokenOwnerUserID,
		arg.TokenOwnedByWallets,
		arg.TokenOwnedByWalletsStartIdx,
		arg.TokenOwnedByWalletsEndIdx,
		arg.TokenIsCreatorToken,
		arg.TokenTokenID,
		arg.TokenContractAddress,
		arg.TokenChain,
		arg.TokenContractID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []UpsertTokensRow
	for rows.Next() {
		var i UpsertTokensRow
		if err := rows.Scan(
			&i.Token.ID,
			&i.Token.Deleted,
			&i.Token.Version,
			&i.Token.CreatedAt,
			&i.Token.LastUpdated,
			&i.Token.NameDeprecated,
			&i.Token.DescriptionDeprecated,
			&i.Token.CollectorsNote,
			&i.Token.TokenTypeDeprecated,
			&i.Token.TokenIDDeprecated,
			&i.Token.Quantity,
			&i.Token.OwnershipHistoryDeprecated,
			&i.Token.ExternalUrlDeprecated,
			&i.Token.BlockNumber,
			&i.Token.OwnerUserID,
			&i.Token.OwnedByWallets,
			&i.Token.ChainDeprecated,
			&i.Token.ContractID,
			&i.Token.IsUserMarkedSpam,
			&i.Token.IsProviderMarkedSpamDeprecated,
			&i.Token.LastSynced,
			&i.Token.TokenUriDeprecated,
			&i.Token.FallbackMediaDeprecated,
			&i.Token.TokenMediaIDDeprecated,
			&i.Token.IsCreatorToken,
			&i.Token.TokenDefinitionID,
			&i.Token.IsHolderToken,
			&i.Token.Displayable,
			&i.TokenDefinition.ID,
			&i.TokenDefinition.CreatedAt,
			&i.TokenDefinition.LastUpdated,
			&i.TokenDefinition.Deleted,
			&i.TokenDefinition.Name,
			&i.TokenDefinition.Description,
			&i.TokenDefinition.TokenType,
			&i.TokenDefinition.TokenID,
			&i.TokenDefinition.ExternalUrl,
			&i.TokenDefinition.Chain,
			&i.TokenDefinition.Metadata,
			&i.TokenDefinition.FallbackMedia,
			&i.TokenDefinition.ContractAddress,
			&i.TokenDefinition.ContractID,
			&i.TokenDefinition.TokenMediaID,
			&i.Contract.ID,
			&i.Contract.Deleted,
			&i.Contract.Version,
			&i.Contract.CreatedAt,
			&i.Contract.LastUpdated,
			&i.Contract.Name,
			&i.Contract.Symbol,
			&i.Contract.Address,
			&i.Contract.CreatorAddress,
			&i.Contract.Chain,
			&i.Contract.ProfileBannerUrl,
			&i.Contract.ProfileImageUrl,
			&i.Contract.BadgeUrl,
			&i.Contract.Description,
			&i.Contract.OwnerAddress,
			&i.Contract.IsProviderMarkedSpam,
			&i.Contract.ParentID,
			&i.Contract.OverrideCreatorUserID,
			&i.Contract.L1Chain,
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
