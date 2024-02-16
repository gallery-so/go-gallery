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
  , name = excluded.name
  , owner_address =
      case
          when nullif(contracts.owner_address, '') is null or (@can_overwrite_owner_address::bool and nullif (excluded.owner_address, '') is not null)
            then excluded.owner_address
          else
            contracts.owner_address
      end
  , description = excluded.description
  , profile_image_url = excluded.profile_image_url
  , deleted = excluded.deleted
  , last_updated = now()
returning *;
