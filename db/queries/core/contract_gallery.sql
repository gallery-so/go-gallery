-- name: UpsertContracts :many
insert into contracts (id, deleted, version, created_at, address, symbol, name, owner_address, chain, description) (
  select unnest(@id::varchar[])
  , unnest(@deleted::boolean[])
  , unnest(@version::int[])
  , unnest(@created_at::timestamptz[])
  , unnest(@address::varchar[])
  , unnest(@symbol::varchar[])
  , unnest(@name::varchar[])
  , unnest(@owner_address::varchar[])
  , unnest(@chain::int[])
  , unnest(@description::varchar[])
)
on conflict (chain, address) where parent_id is null
do update set symbol = excluded.symbol
  , version = excluded.version
  , name = excluded.name
  , owner_address = excluded.owner_address
  , description = excluded.description
  , deleted = excluded.deleted
  , last_updated = now()
returning *;

-- name: UpsertCreatedTokens :many
with parent_contracts_data(id, deleted, created_at, name, symbol, address, creator_address, chain, description) as (
  select unnest(@parent_contract_id::varchar[]) as id
    , unnest(@parent_contract_deleted::boolean[]) as deleted
    , unnest(@parent_contract_created_at::timestamptz[]) as created_at
    , unnest(@parent_contract_name::varchar[]) as name
    , unnest(@parent_contract_symbol::varchar[]) as symbol
    , unnest(@parent_contract_address::varchar[]) as address
    , unnest(@parent_contract_creator_address::varchar[]) as creator_address
    , unnest(@parent_contract_chain::int[]) as chain
    , unnest(@parent_contract_description::varchar[]) as description
)
, child_contracts_data(id, deleted, created_at, name, address, creator_address, chain, description, parent_address) as (
  select unnest(@child_contract_id::varchar[]) as id
    , unnest(@child_contract_deleted::boolean[]) as deleted
    , unnest(@child_contract_created_at::timestamptz[]) as created_at
    , unnest(@child_contract_name::varchar[]) as name
    , unnest(@child_contract_address::varchar[]) as address
    , unnest(@child_contract_creator_address::varchar[]) as creator_address
    , unnest(@child_contract_chain::int[]) as chain
    , unnest(@child_contract_description::varchar[]) as description
    , unnest(@child_contract_parent_address::varchar[]) as parent_address
), insert_parent_contracts as (
  insert into contracts(id, deleted, created_at, name, symbol, address, creator_address, chain, description)
  (
    select id
      , deleted
      , created_at
      , name
      , symbol
      , address
      , creator_address
      , chain
      , description
    from parent_contracts_data
  )
  on conflict (chain, address) where parent_id is null
  do update set deleted = excluded.deleted
    , name = excluded.name
    , symbol = excluded.symbol
    , creator_address = excluded.creator_address
    , description = excluded.description
    , last_updated = now()
  returning *
)
insert into contracts(id, deleted, created_at, name, address, creator_address, chain, description, parent_id)
(
  select child.id
    , child.deleted
    , child.created_at
    , child.name
    , child.address
    , child.creator_address
    , child.chain
    , child.description
    , insert_parent_contracts.id
  from child_contracts_data child
  join insert_parent_contracts on child.chain = insert_parent_contracts.chain and child.parent_address = insert_parent_contracts.address
)
on conflict (chain, parent_id, address) where parent_id is not null
do update set deleted = excluded.deleted
  , name = excluded.name
  , creator_address = excluded.creator_address
  , description = excluded.description
  , last_updated = now()
returning *;
