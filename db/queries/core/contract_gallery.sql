-- name: UpsertParentContracts :many
insert into contracts(id, deleted, version, created_at, address, symbol, name, owner_address, chain, l1_chain, description, profile_image_url, is_provider_marked_spam) (
  select unnest(@ids::varchar[])
    , false
    , unnest(@version::int[])
    , now()
    , unnest(@address::varchar[])
    , unnest(@symbol::varchar[])
    , unnest(@name::varchar[])
    , unnest(@owner_address::varchar[])
    , unnest(@chain::int[])
    , unnest(@l1_chain::int[])
    , unnest(@description::varchar[])
    , unnest(@profile_image_url::varchar[])
    , unnest(@provider_marked_spam::bool[])
)
on conflict (l1_chain, chain, address) where parent_id is null
do update set symbol = coalesce(nullif(excluded.symbol, ''), nullif(contracts.symbol, ''))
  , version = excluded.version
  , name = coalesce(nullif(excluded.name, ''), nullif(contracts.name, ''))
  , owner_address =
      case
          when nullif(contracts.owner_address, '') is null or (@can_overwrite_owner_address::bool and nullif (excluded.owner_address, '') is not null)
            then excluded.owner_address
          else
            contracts.owner_address
      end
  , description = coalesce(nullif(excluded.description, ''), nullif(contracts.description, ''))
  , profile_image_url = coalesce(nullif(excluded.profile_image_url, ''), nullif(contracts.profile_image_url, ''))
  , deleted = excluded.deleted
  , last_updated = now()
returning *;

-- name: UpsertChildContracts :many
insert into contracts(id, deleted, version, created_at, name, address, creator_address, owner_address, chain, l1_chain, description, parent_id) (
  select unnest(@id::varchar[]) as id
    , false
    , 0
    , now()
    , unnest(@name::varchar[])
    , unnest(@address::varchar[])
    , unnest(@creator_address::varchar[])
    , unnest(@owner_address::varchar[])
    , unnest(@chain::int[])
    , unnest(@l1_chain::int[])
    , unnest(@description::varchar[])
    , unnest(@parent_ids::varchar[])
)
on conflict (l1_chain, chain, parent_id, address) where parent_id is not null
do update set deleted = excluded.deleted
  , name = excluded.name
  , creator_address = excluded.creator_address
  , owner_address = excluded.owner_address
  , description = excluded.description
  , last_updated = now()
returning *;
