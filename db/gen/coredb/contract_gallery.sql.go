// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.18.0
// source: contract_gallery.sql

package coredb

import (
	"context"
)

const upsertChildContracts = `-- name: UpsertChildContracts :many
insert into contracts(id, deleted, version, created_at, name, address, creator_address, owner_address, chain, l1_chain, description, parent_id) (
  select unnest($1::varchar[]) as id
    , false
    , 0
    , now()
    , unnest($2::varchar[])
    , unnest($3::varchar[])
    , unnest($4::varchar[])
    , unnest($5::varchar[])
    , unnest($6::int[])
    , unnest($7::int[])
    , unnest($8::varchar[])
    , unnest($9::varchar[])
)
on conflict (l1_chain, chain, parent_id, address) where parent_id is not null
do update set deleted = excluded.deleted
  , name = excluded.name
  , creator_address = excluded.creator_address
  , owner_address = excluded.owner_address
  , description = excluded.description
  , last_updated = now()
returning id, deleted, version, created_at, last_updated, name, symbol, address, creator_address, chain, profile_banner_url, profile_image_url, badge_url, description, owner_address, is_provider_marked_spam, parent_id, override_creator_user_id, l1_chain
`

type UpsertChildContractsParams struct {
	ID             []string `json:"id"`
	Name           []string `json:"name"`
	Address        []string `json:"address"`
	CreatorAddress []string `json:"creator_address"`
	OwnerAddress   []string `json:"owner_address"`
	Chain          []int32  `json:"chain"`
	L1Chain        []int32  `json:"l1_chain"`
	Description    []string `json:"description"`
	ParentIds      []string `json:"parent_ids"`
}

func (q *Queries) UpsertChildContracts(ctx context.Context, arg UpsertChildContractsParams) ([]Contract, error) {
	rows, err := q.db.Query(ctx, upsertChildContracts,
		arg.ID,
		arg.Name,
		arg.Address,
		arg.CreatorAddress,
		arg.OwnerAddress,
		arg.Chain,
		arg.L1Chain,
		arg.Description,
		arg.ParentIds,
	)
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
			&i.ProfileBannerUrl,
			&i.ProfileImageUrl,
			&i.BadgeUrl,
			&i.Description,
			&i.OwnerAddress,
			&i.IsProviderMarkedSpam,
			&i.ParentID,
			&i.OverrideCreatorUserID,
			&i.L1Chain,
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

const upsertParentContracts = `-- name: UpsertParentContracts :many
insert into contracts(id, deleted, version, created_at, address, symbol, name, owner_address, chain, l1_chain, description, profile_image_url, is_provider_marked_spam) (
  select unnest($1::varchar[])
    , false
    , unnest($2::int[])
    , now()
    , unnest($3::varchar[])
    , unnest($4::varchar[])
    , unnest($5::varchar[])
    , unnest($6::varchar[])
    , unnest($7::int[])
    , unnest($8::int[])
    , unnest($9::varchar[])
    , unnest($10::varchar[])
    , unnest($11::bool[])
)
on conflict (l1_chain, chain, address) where parent_id is null
do update set symbol = coalesce(nullif(excluded.symbol, ''), nullif(contracts.symbol, ''))
  , version = excluded.version
  , name = coalesce(nullif(excluded.name, ''), nullif(contracts.name, ''))
  , owner_address =
      case
          when nullif(contracts.owner_address, '') is null or ($12::bool and nullif (excluded.owner_address, '') is not null)
            then excluded.owner_address
          else
            contracts.owner_address
      end
  , description = coalesce(nullif(excluded.description, ''), nullif(contracts.description, ''))
  , profile_image_url = coalesce(nullif(excluded.profile_image_url, ''), nullif(contracts.profile_image_url, ''))
  , deleted = excluded.deleted
  , last_updated = now()
returning id, deleted, version, created_at, last_updated, name, symbol, address, creator_address, chain, profile_banner_url, profile_image_url, badge_url, description, owner_address, is_provider_marked_spam, parent_id, override_creator_user_id, l1_chain
`

type UpsertParentContractsParams struct {
	Ids                      []string `json:"ids"`
	Version                  []int32  `json:"version"`
	Address                  []string `json:"address"`
	Symbol                   []string `json:"symbol"`
	Name                     []string `json:"name"`
	OwnerAddress             []string `json:"owner_address"`
	Chain                    []int32  `json:"chain"`
	L1Chain                  []int32  `json:"l1_chain"`
	Description              []string `json:"description"`
	ProfileImageUrl          []string `json:"profile_image_url"`
	ProviderMarkedSpam       []bool   `json:"provider_marked_spam"`
	CanOverwriteOwnerAddress bool     `json:"can_overwrite_owner_address"`
}

func (q *Queries) UpsertParentContracts(ctx context.Context, arg UpsertParentContractsParams) ([]Contract, error) {
	rows, err := q.db.Query(ctx, upsertParentContracts,
		arg.Ids,
		arg.Version,
		arg.Address,
		arg.Symbol,
		arg.Name,
		arg.OwnerAddress,
		arg.Chain,
		arg.L1Chain,
		arg.Description,
		arg.ProfileImageUrl,
		arg.ProviderMarkedSpam,
		arg.CanOverwriteOwnerAddress,
	)
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
			&i.ProfileBannerUrl,
			&i.ProfileImageUrl,
			&i.BadgeUrl,
			&i.Description,
			&i.OwnerAddress,
			&i.IsProviderMarkedSpam,
			&i.ParentID,
			&i.OverrideCreatorUserID,
			&i.L1Chain,
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
