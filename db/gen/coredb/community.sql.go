// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.18.0
// source: community.sql

package coredb

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
)

const countHoldersByCommunityID = `-- name: CountHoldersByCommunityID :one
with community_data as (
    select community_type, contract_id
    from communities
    where communities.id = $1 and not deleted
),

community_tokens as (
    select tokens.id, tokens.deleted, tokens.version, tokens.created_at, tokens.last_updated, tokens.collectors_note, tokens.quantity, tokens.block_number, tokens.owner_user_id, tokens.owned_by_wallets, tokens.contract_id, tokens.is_user_marked_spam, tokens.last_synced, tokens.is_creator_token, tokens.token_definition_id, tokens.is_holder_token, tokens.displayable
    from community_data, tokens
    where community_data.community_type = 0
        and tokens.contract_id = community_data.contract_id
        and not tokens.deleted

    union all

    select tokens.id, tokens.deleted, tokens.version, tokens.created_at, tokens.last_updated, tokens.collectors_note, tokens.quantity, tokens.block_number, tokens.owner_user_id, tokens.owned_by_wallets, tokens.contract_id, tokens.is_user_marked_spam, tokens.last_synced, tokens.is_creator_token, tokens.token_definition_id, tokens.is_holder_token, tokens.displayable
    from community_data, tokens
        join token_community_memberships on tokens.token_definition_id = token_community_memberships.token_definition_id
            and token_community_memberships.community_id = $1
            and not token_community_memberships.deleted
    where community_data.community_type != 0
        and not tokens.deleted
)

select count(distinct u.id) from users u, community_tokens t
    where t.owner_user_id = u.id
    and t.displayable
    and u.universal = false
    and t.deleted = false and u.deleted = false
`

func (q *Queries) CountHoldersByCommunityID(ctx context.Context, communityID persist.DBID) (int64, error) {
	row := q.db.QueryRow(ctx, countHoldersByCommunityID, communityID)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const countPostsByCommunityID = `-- name: CountPostsByCommunityID :one

with community_data as (
    select community_type, contract_id
    from communities
    where communities.id = $1 and not deleted
),

community_posts as (
    (
        select posts.id, posts.version, posts.token_ids, posts.contract_ids, posts.actor_id, posts.caption, posts.created_at, posts.last_updated, posts.deleted, posts.is_first_post, posts.user_mint_url
            from community_data, posts
            where community_data.community_type = 0
                and community_data.contract_id = any(posts.contract_ids)
                and posts.deleted = false
    )

    union all

    (
        select posts.id, posts.version, posts.token_ids, posts.contract_ids, posts.actor_id, posts.caption, posts.created_at, posts.last_updated, posts.deleted, posts.is_first_post, posts.user_mint_url
            from community_data, posts
                join tokens on posts.token_ids @> array[tokens.id] and not tokens.deleted
                join token_community_memberships on tokens.token_definition_id = token_community_memberships.token_definition_id
                    and token_community_memberships.community_id = $1
                    and not token_community_memberships.deleted
            where community_data.community_type = 1
              and posts.deleted = false
    )
)

select count(*) from community_posts
`

// set role to access_rw;
// create index posts_token_ids_idx on posts using gin (token_ids) where (deleted = false);
// drop index if exists posts_token_ids_idx;
func (q *Queries) CountPostsByCommunityID(ctx context.Context, communityID persist.DBID) (int64, error) {
	row := q.db.QueryRow(ctx, countPostsByCommunityID, communityID)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const countTokensByCommunityID = `-- name: CountTokensByCommunityID :one
with community_data as (
    select community_type, contract_id
    from communities
    where communities.id = $1 and not deleted
),

community_tokens as (
    select tokens.id, tokens.deleted, tokens.version, tokens.created_at, tokens.last_updated, tokens.collectors_note, tokens.quantity, tokens.block_number, tokens.owner_user_id, tokens.owned_by_wallets, tokens.contract_id, tokens.is_user_marked_spam, tokens.last_synced, tokens.is_creator_token, tokens.token_definition_id, tokens.is_holder_token, tokens.displayable
    from community_data, tokens
    where community_data.community_type = 0
        and tokens.contract_id = community_data.contract_id
        and not tokens.deleted

    union all

    select tokens.id, tokens.deleted, tokens.version, tokens.created_at, tokens.last_updated, tokens.collectors_note, tokens.quantity, tokens.block_number, tokens.owner_user_id, tokens.owned_by_wallets, tokens.contract_id, tokens.is_user_marked_spam, tokens.last_synced, tokens.is_creator_token, tokens.token_definition_id, tokens.is_holder_token, tokens.displayable
    from community_data, tokens
        join token_community_memberships on tokens.token_definition_id = token_community_memberships.token_definition_id
            and token_community_memberships.community_id = $1
            and not token_community_memberships.deleted
    where community_data.community_type != 0
        and not tokens.deleted
)

select count(t.*) from community_tokens t
    join token_definitions td on t.token_definition_id = td.id
    join users u on u.id = t.owner_user_id
    join contracts c on t.contract_id = c.id
    where t.displayable
    and t.deleted = false
    and c.deleted = false
    and td.deleted = false
    and u.universal = false
`

func (q *Queries) CountTokensByCommunityID(ctx context.Context, communityID persist.DBID) (int64, error) {
	row := q.db.QueryRow(ctx, countTokensByCommunityID, communityID)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const getCommunitiesByKeys = `-- name: GetCommunitiesByKeys :many
with keys as (
    select unnest ($1::int[]) as type
         , unnest ($2::varchar[]) as key1
         , unnest ($3::varchar[]) as key2
         , unnest ($4::varchar[]) as key3
         , unnest ($5::varchar[]) as key4
         , generate_subscripts($1::varchar[], 1) as batch_key_index
)
select k.batch_key_index, c.id, c.version, c.community_type, c.key1, c.key2, c.key3, c.key4, c.name, c.override_name, c.description, c.override_description, c.profile_image_url, c.override_profile_image_url, c.badge_url, c.override_badge_url, c.contract_id, c.created_at, c.last_updated, c.deleted, c.website_url, c.override_website_url from keys k
    join communities c on
        k.type = c.community_type
        and k.key1 = c.key1
        and k.key2 = c.key2
        and k.key3 = c.key3
        and k.key4 = c.key4
    where not c.deleted
`

type GetCommunitiesByKeysParams struct {
	Types []int32  `db:"types" json:"types"`
	Key1  []string `db:"key1" json:"key1"`
	Key2  []string `db:"key2" json:"key2"`
	Key3  []string `db:"key3" json:"key3"`
	Key4  []string `db:"key4" json:"key4"`
}

type GetCommunitiesByKeysRow struct {
	BatchKeyIndex int32     `db:"batch_key_index" json:"batch_key_index"`
	Community     Community `db:"community" json:"community"`
}

// dataloader-config: skip=true
// Get communities by keys
func (q *Queries) GetCommunitiesByKeys(ctx context.Context, arg GetCommunitiesByKeysParams) ([]GetCommunitiesByKeysRow, error) {
	rows, err := q.db.Query(ctx, getCommunitiesByKeys,
		arg.Types,
		arg.Key1,
		arg.Key2,
		arg.Key3,
		arg.Key4,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetCommunitiesByKeysRow
	for rows.Next() {
		var i GetCommunitiesByKeysRow
		if err := rows.Scan(
			&i.BatchKeyIndex,
			&i.Community.ID,
			&i.Community.Version,
			&i.Community.CommunityType,
			&i.Community.Key1,
			&i.Community.Key2,
			&i.Community.Key3,
			&i.Community.Key4,
			&i.Community.Name,
			&i.Community.OverrideName,
			&i.Community.Description,
			&i.Community.OverrideDescription,
			&i.Community.ProfileImageUrl,
			&i.Community.OverrideProfileImageUrl,
			&i.Community.BadgeUrl,
			&i.Community.OverrideBadgeUrl,
			&i.Community.ContractID,
			&i.Community.CreatedAt,
			&i.Community.LastUpdated,
			&i.Community.Deleted,
			&i.Community.WebsiteUrl,
			&i.Community.OverrideWebsiteUrl,
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

const getCommunityByID = `-- name: GetCommunityByID :one
select id, version, community_type, key1, key2, key3, key4, name, override_name, description, override_description, profile_image_url, override_profile_image_url, badge_url, override_badge_url, contract_id, created_at, last_updated, deleted, website_url, override_website_url from communities
    where id = $1
        and not deleted
`

func (q *Queries) GetCommunityByID(ctx context.Context, id persist.DBID) (Community, error) {
	row := q.db.QueryRow(ctx, getCommunityByID, id)
	var i Community
	err := row.Scan(
		&i.ID,
		&i.Version,
		&i.CommunityType,
		&i.Key1,
		&i.Key2,
		&i.Key3,
		&i.Key4,
		&i.Name,
		&i.OverrideName,
		&i.Description,
		&i.OverrideDescription,
		&i.ProfileImageUrl,
		&i.OverrideProfileImageUrl,
		&i.BadgeUrl,
		&i.OverrideBadgeUrl,
		&i.ContractID,
		&i.CreatedAt,
		&i.LastUpdated,
		&i.Deleted,
		&i.WebsiteUrl,
		&i.OverrideWebsiteUrl,
	)
	return i, err
}

const getCommunityContractProviders = `-- name: GetCommunityContractProviders :many
select id, version, contract_id, community_type, is_valid_provider, created_at, last_updated, deleted from community_contract_providers
    where contract_id = any($1)
    and not deleted
`

func (q *Queries) GetCommunityContractProviders(ctx context.Context, contractIds persist.DBIDList) ([]CommunityContractProvider, error) {
	rows, err := q.db.Query(ctx, getCommunityContractProviders, contractIds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CommunityContractProvider
	for rows.Next() {
		var i CommunityContractProvider
		if err := rows.Scan(
			&i.ID,
			&i.Version,
			&i.ContractID,
			&i.CommunityType,
			&i.IsValidProvider,
			&i.CreatedAt,
			&i.LastUpdated,
			&i.Deleted,
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

const isMemberOfCommunity = `-- name: IsMemberOfCommunity :one
with community_data as (
    select community_type, contract_id
    from communities
    where communities.id = $2 and not deleted
),

community_token_definitions as (
    select td.id, td.created_at, td.last_updated, td.deleted, td.name, td.description, td.token_type, td.token_id, td.external_url, td.chain, td.metadata, td.fallback_media, td.contract_address, td.contract_id, td.token_media_id
    from community_data, token_definitions td
    where community_data.community_type = 0
        and td.contract_id = community_data.contract_id
        and not td.deleted

    union all

    select td.id, td.created_at, td.last_updated, td.deleted, td.name, td.description, td.token_type, td.token_id, td.external_url, td.chain, td.metadata, td.fallback_media, td.contract_address, td.contract_id, td.token_media_id
    from community_data, token_definitions td
        join token_community_memberships on td.id = token_community_memberships.token_definition_id
            and token_community_memberships.community_id = $2
            and not token_community_memberships.deleted
    where community_data.community_type != 0
        and not td.deleted
)

select exists(
    select 1
    from tokens, community_token_definitions
    where tokens.owner_user_id = $1
      and not tokens.deleted
      and tokens.displayable
      and tokens.token_definition_id = community_token_definitions.id
)
`

type IsMemberOfCommunityParams struct {
	UserID      persist.DBID `db:"user_id" json:"user_id"`
	CommunityID persist.DBID `db:"community_id" json:"community_id"`
}

func (q *Queries) IsMemberOfCommunity(ctx context.Context, arg IsMemberOfCommunityParams) (bool, error) {
	row := q.db.QueryRow(ctx, isMemberOfCommunity, arg.UserID, arg.CommunityID)
	var exists bool
	err := row.Scan(&exists)
	return exists, err
}

const upsertCommunities = `-- name: UpsertCommunities :many
insert into communities(id, version, name, description, community_type, key1, key2, key3, key4, profile_image_url, badge_url, website_url, contract_id, created_at, last_updated, deleted) (
    select unnest($1::varchar[])
         , unnest($2::int[])
         , unnest($3::varchar[])
         , unnest($4::varchar[])
         , unnest($5::int[])
         , unnest($6::varchar[])
         , unnest($7::varchar[])
         , unnest($8::varchar[])
         , unnest($9::varchar[])
         , nullif(unnest($10::varchar[]), '')
         , nullif(unnest($11::varchar[]), '')
         , nullif(unnest($12::varchar[]), '')
         , nullif(unnest($13::varchar[]), '')
         , now()
         , now()
         , false
)
on conflict (community_type, key1, key2, key3, key4) where not deleted
    do update set version = excluded.version
                , name = coalesce(nullif(excluded.name, ''), nullif(communities.name, ''), '')
                , description = coalesce(nullif(excluded.description, ''), nullif(communities.description, ''), '')
                , profile_image_url = coalesce(nullif(excluded.profile_image_url, ''), nullif(communities.profile_image_url, ''))
                , badge_url = coalesce(nullif(excluded.badge_url, ''), nullif(communities.badge_url, ''))
                , website_url = coalesce(nullif(excluded.website_url, ''), nullif(communities.website_url, ''))
                , contract_id = coalesce(nullif(excluded.contract_id, ''), nullif(communities.contract_id, ''))
                , last_updated = now()
                , deleted = excluded.deleted
returning id, version, community_type, key1, key2, key3, key4, name, override_name, description, override_description, profile_image_url, override_profile_image_url, badge_url, override_badge_url, contract_id, created_at, last_updated, deleted, website_url, override_website_url
`

type UpsertCommunitiesParams struct {
	Ids             []string `db:"ids" json:"ids"`
	Version         []int32  `db:"version" json:"version"`
	Name            []string `db:"name" json:"name"`
	Description     []string `db:"description" json:"description"`
	CommunityType   []int32  `db:"community_type" json:"community_type"`
	Key1            []string `db:"key1" json:"key1"`
	Key2            []string `db:"key2" json:"key2"`
	Key3            []string `db:"key3" json:"key3"`
	Key4            []string `db:"key4" json:"key4"`
	ProfileImageUrl []string `db:"profile_image_url" json:"profile_image_url"`
	BadgeUrl        []string `db:"badge_url" json:"badge_url"`
	WebsiteUrl      []string `db:"website_url" json:"website_url"`
	ContractID      []string `db:"contract_id" json:"contract_id"`
}

func (q *Queries) UpsertCommunities(ctx context.Context, arg UpsertCommunitiesParams) ([]Community, error) {
	rows, err := q.db.Query(ctx, upsertCommunities,
		arg.Ids,
		arg.Version,
		arg.Name,
		arg.Description,
		arg.CommunityType,
		arg.Key1,
		arg.Key2,
		arg.Key3,
		arg.Key4,
		arg.ProfileImageUrl,
		arg.BadgeUrl,
		arg.WebsiteUrl,
		arg.ContractID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Community
	for rows.Next() {
		var i Community
		if err := rows.Scan(
			&i.ID,
			&i.Version,
			&i.CommunityType,
			&i.Key1,
			&i.Key2,
			&i.Key3,
			&i.Key4,
			&i.Name,
			&i.OverrideName,
			&i.Description,
			&i.OverrideDescription,
			&i.ProfileImageUrl,
			&i.OverrideProfileImageUrl,
			&i.BadgeUrl,
			&i.OverrideBadgeUrl,
			&i.ContractID,
			&i.CreatedAt,
			&i.LastUpdated,
			&i.Deleted,
			&i.WebsiteUrl,
			&i.OverrideWebsiteUrl,
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

const upsertCommunityContractProviders = `-- name: UpsertCommunityContractProviders :exec
with entries as (
    select unnest($1::varchar[]) as id
         , unnest($2::varchar[]) as contract_id
         , unnest($3::int[]) as community_type
         , unnest($4::bool[]) as is_valid_provider
         , now() as created_at
         , now() as last_updated
         , false as deleted
)
insert into community_contract_providers(id, contract_id, community_type, is_valid_provider, created_at, last_updated, deleted) (
    select id, contract_id, community_type, is_valid_provider, created_at, last_updated, deleted from entries
)
on conflict (contract_id, community_type) where not deleted
    do update set is_valid_provider = excluded.is_valid_provider
                , last_updated = now()
returning id, version, contract_id, community_type, is_valid_provider, created_at, last_updated, deleted
`

type UpsertCommunityContractProvidersParams struct {
	Ids             []string `db:"ids" json:"ids"`
	ContractID      []string `db:"contract_id" json:"contract_id"`
	CommunityType   []int32  `db:"community_type" json:"community_type"`
	IsValidProvider []bool   `db:"is_valid_provider" json:"is_valid_provider"`
}

func (q *Queries) UpsertCommunityContractProviders(ctx context.Context, arg UpsertCommunityContractProvidersParams) error {
	_, err := q.db.Exec(ctx, upsertCommunityContractProviders,
		arg.Ids,
		arg.ContractID,
		arg.CommunityType,
		arg.IsValidProvider,
	)
	return err
}

const upsertCommunityCreators = `-- name: UpsertCommunityCreators :many
with entries as (
    select unnest($1::varchar[]) as id
         , unnest($2::varchar[]) as community_id
         , unnest($3::int[]) as creator_type
         , nullif(unnest($4::varchar[]), '') as creator_user_id
         , nullif(unnest($5::varchar[]), '') as creator_address
         , unnest($6::int[]) as creator_address_chain
         , unnest($7::int[]) as creator_address_l1_chain
         , now() as created_at
         , now() as last_updated
         , false as deleted
)

insert into community_creators(id, community_id, creator_type, creator_user_id, creator_address, creator_address_chain, creator_address_l1_chain, created_at, last_updated, deleted) (
    select id, community_id, creator_type, creator_user_id, creator_address, creator_address_chain, creator_address_l1_chain, created_at, last_updated, deleted from entries
)
on conflict do nothing
returning id, version, creator_type, community_id, creator_user_id, creator_address, creator_address_l1_chain, creator_address_chain, created_at, last_updated, deleted
`

type UpsertCommunityCreatorsParams struct {
	Ids                   []string `db:"ids" json:"ids"`
	CommunityID           []string `db:"community_id" json:"community_id"`
	CreatorType           []int32  `db:"creator_type" json:"creator_type"`
	CreatorUserID         []string `db:"creator_user_id" json:"creator_user_id"`
	CreatorAddress        []string `db:"creator_address" json:"creator_address"`
	CreatorAddressChain   []int32  `db:"creator_address_chain" json:"creator_address_chain"`
	CreatorAddressL1Chain []int32  `db:"creator_address_l1_chain" json:"creator_address_l1_chain"`
}

func (q *Queries) UpsertCommunityCreators(ctx context.Context, arg UpsertCommunityCreatorsParams) ([]CommunityCreator, error) {
	rows, err := q.db.Query(ctx, upsertCommunityCreators,
		arg.Ids,
		arg.CommunityID,
		arg.CreatorType,
		arg.CreatorUserID,
		arg.CreatorAddress,
		arg.CreatorAddressChain,
		arg.CreatorAddressL1Chain,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CommunityCreator
	for rows.Next() {
		var i CommunityCreator
		if err := rows.Scan(
			&i.ID,
			&i.Version,
			&i.CreatorType,
			&i.CommunityID,
			&i.CreatorUserID,
			&i.CreatorAddress,
			&i.CreatorAddressL1Chain,
			&i.CreatorAddressChain,
			&i.CreatedAt,
			&i.LastUpdated,
			&i.Deleted,
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

const upsertContractCommunityMemberships = `-- name: UpsertContractCommunityMemberships :many
with memberships as (
    select unnest($1::varchar[]) as id
         , unnest($2::varchar[]) as contract_id
         , unnest($3::varchar[]) as community_id
         , now() as created_at
         , now() as last_updated
         , false as deleted
),
valid_memberships as (
    select memberships.id, memberships.contract_id, memberships.community_id, memberships.created_at, memberships.last_updated, memberships.deleted
    from memberships
    join communities on communities.id = memberships.community_id and not communities.deleted
    join contracts on contracts.id = memberships.contract_id and not contracts.deleted
)
insert into contract_community_memberships(id, contract_id, community_id, created_at, last_updated, deleted) (
    select id, contract_id, community_id, created_at, last_updated, deleted from valid_memberships
)
on conflict (community_id, contract_id) where not deleted
    do nothing
returning id, version, contract_id, community_id, created_at, last_updated, deleted
`

type UpsertContractCommunityMembershipsParams struct {
	Ids         []string `db:"ids" json:"ids"`
	ContractID  []string `db:"contract_id" json:"contract_id"`
	CommunityID []string `db:"community_id" json:"community_id"`
}

func (q *Queries) UpsertContractCommunityMemberships(ctx context.Context, arg UpsertContractCommunityMembershipsParams) ([]ContractCommunityMembership, error) {
	rows, err := q.db.Query(ctx, upsertContractCommunityMemberships, arg.Ids, arg.ContractID, arg.CommunityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ContractCommunityMembership
	for rows.Next() {
		var i ContractCommunityMembership
		if err := rows.Scan(
			&i.ID,
			&i.Version,
			&i.ContractID,
			&i.CommunityID,
			&i.CreatedAt,
			&i.LastUpdated,
			&i.Deleted,
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

const upsertTokenCommunityMemberships = `-- name: UpsertTokenCommunityMemberships :many
with memberships as (
    select unnest($1::varchar[]) as id
         , unnest($2::varchar[]) as token_definition_id
         , unnest($3::varchar[]) as community_id
         , now() as created_at
         , now() as last_updated
         , false as deleted
),
valid_memberships as (
    select memberships.id, memberships.token_definition_id, memberships.community_id, memberships.created_at, memberships.last_updated, memberships.deleted
    from memberships
    join communities on communities.id = memberships.community_id and not communities.deleted
    join token_definitions on token_definitions.id = memberships.token_definition_id and not token_definitions.deleted
)
insert into token_community_memberships(id, token_definition_id, community_id, created_at, last_updated, deleted) (
    select id, token_definition_id, community_id, created_at, last_updated, deleted from valid_memberships
)
on conflict (community_id, token_definition_id) where not deleted
    do nothing
returning id, version, token_definition_id, community_id, created_at, last_updated, deleted
`

type UpsertTokenCommunityMembershipsParams struct {
	Ids               []string `db:"ids" json:"ids"`
	TokenDefinitionID []string `db:"token_definition_id" json:"token_definition_id"`
	CommunityID       []string `db:"community_id" json:"community_id"`
}

func (q *Queries) UpsertTokenCommunityMemberships(ctx context.Context, arg UpsertTokenCommunityMembershipsParams) ([]TokenCommunityMembership, error) {
	rows, err := q.db.Query(ctx, upsertTokenCommunityMemberships, arg.Ids, arg.TokenDefinitionID, arg.CommunityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []TokenCommunityMembership
	for rows.Next() {
		var i TokenCommunityMembership
		if err := rows.Scan(
			&i.ID,
			&i.Version,
			&i.TokenDefinitionID,
			&i.CommunityID,
			&i.CreatedAt,
			&i.LastUpdated,
			&i.Deleted,
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
