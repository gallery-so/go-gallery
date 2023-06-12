-- name: UpsertContracts :many
insert into contracts (id, deleted, version, created_at, address, symbol, name, owner_address, chain, description, creator_address, parent_contract_id) (
  select unnest(id), false, unnest(version), now(), unnest(address), unnest(symbol), unnest(name), unnest(owner_address), unnest(chain), unnest(description), unnest(creator_address), unnest(parent_id)
  from (select @ids as id
    , @version::int[] as version
    , @address::varchar[] as address
    , @symbol::varchar[] as symbol
    , @name::varchar[] as name
    , @owner_address::varchar[] as owner_address
    , @chain::int[] as chain
    , @description::varchar[] as description
    , @creator_address::varchar[] as creator_address
    , @parent_ids as parent_id) params
)
on conflict (chain, address) where parent_id is null
do update set symbol = excluded.symbol
  , version = excluded.version
  , name = excluded.name
  , owner_address = excluded.owner_address
  , description = excluded.description
  , deleted = excluded.deleted
  , last_updated = now()
  , creator_address = case
    when contracts.creator_address is not null and excluded.creator_address is null then contracts.creator_address
    else excluded.creator_address
    end
  , parent_id = case
    when contracts.parent_id is not null and excluded.parent_id is null then contracts.parent_id
    else excluded.parent_id
    end
returning *;
