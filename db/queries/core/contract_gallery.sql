-- name: UpsertContracts :many
insert into contracts (id, deleted, version, created_at, address, symbol, name, owner_address, chain, description) (
  select unnest(id), false, unnest(version), now(), unnest(address), unnest(symbol), unnest(name), unnest(owner_address), unnest(chain), unnest(description)
  from (select @ids as id
    , @version::int[] as version
    , @address::varchar[] as address
    , @symbol::varchar[] as symbol
    , @name::varchar[] as name
    , @owner_address::varchar[] as owner_address
    , @chain::int[] as chain
    , @description::varchar[] as description) params
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
-- parent_contracts_data is the parent contract data to be inserted
with parent_contracts_data(id, name, symbol, address, creator_address, chain, description) as (
  select unnest(id), unnest(name), unnest(symbol), unnest(address), unnest(creator_address), unnest(chain), unnest(description)
  from (select @parent_contract_ids as id
    , @parent_contract_name::varchar[] as name
    , @parent_contract_symbol::varchar[] as symbol
    , @parent_contract_address::varchar[] as address
    , @parent_contract_creator_address::varchar[] as creator_address
    , @parent_contract_chain::int[] as chain
    , @parent_contract_description::varchar[] as description) params
),
-- child_contracts_data is the child contract data to be inserted
child_contracts_data(id, name, address, creator_address, chain, description, parent_address) as (
  select unnest(id), unnest(name), unnest(address), unnest(creator_address), unnest(chain), unnest(description), unnest(parent_address)
  from (select @child_contract_ids as id
    , @child_contract_name::varchar[] as name
    , @child_contract_address::varchar[] as address
    , @child_contract_creator_address::varchar[] as creator_address
    , @child_contract_chain::int[] as chain
    , @child_contract_description::varchar[] as description
    , @child_contract_parent_address::varchar[] as parent_address) params
),
-- insert the parent contracts
insert_parent_contracts as (
  insert into contracts(id, deleted, created_at, name, symbol, address, creator_address, chain, description)
  (select id, false, now(), name, symbol, address, creator_address, chain, description from parent_contracts_data)
  on conflict (chain, address) where parent_id is null
  do update set deleted = excluded.deleted
    , name = excluded.name
    , symbol = excluded.symbol
    , creator_address = excluded.creator_address
    , description = excluded.description
    , last_updated = now()
  returning *
)
-- insert the child contracts
insert into contracts(id, deleted, created_at, name, address, creator_address, chain, description, parent_id)
(select child.id, false, now(), child.name, child.address, child.creator_address, child.chain, child.description, insert_parent_contracts.id
  from child_contracts_data child
  join insert_parent_contracts on child.chain = insert_parent_contracts.chain and child.parent_address = insert_parent_contracts.address)
on conflict (chain, parent_id, address) where parent_id is not null
do update set deleted = excluded.deleted
  , name = excluded.name
  , creator_address = excluded.creator_address
  , description = excluded.description
  , last_updated = now()
returning *;
