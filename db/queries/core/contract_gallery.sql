-- name: UpsertContracts :many
insert into contracts (id, deleted, version, created_at, address, symbol, name, creator_address, chain, description) (
  select
  unnest(@id::varchar[])
  , unnest(@deleted::boolean[])
  , unnest(@version::int[])
  , unnest(@created_at::timestamptz[])
  , unnest(@address::varchar[])
  , unnest(@symbol::varchar[])
  , unnest(@name::varchar[])
  , unnest(@creator_address::varchar[])
  , unnest(@chain::int[])
  , unnest(@description::varchar[])
)
on conflict (chain, parent_id, address)
do update set
  symbol = excluded.symbol
  , version = excluded.version
  , name = excluded.name
  , creator_address = excluded.creator_address
  , description = excluded.description
  , deleted = exlucded.deleted
  , last_updated = now()
returning *;

-- name: UpsertCreatedTokens :many
-- UpsertCreatedTokens bulk upserts parent contracts, child contracts and tokens in a single query using data-modifying CTEs.
-- Each data-modifying CTE returns the data that inserted or updated, which is used in the next CTE to insert or update the next table.
-- 
-- This is so that we can:
-- * Reference the id of the parent contract when inserting the child contract
-- * Reference the id of the parent contract and the id of the child contract when inserting the token
-- 
-- parent_contracts_data is the data to be inserted for the parent contracts
with parent_contracts_data(id, deleted, created_at, name, symbol, address, creator_address, chain, description) as (
  select
    unnest(@parent_contract_id::varchar[]) as id
    , unnest(@parent_contract_deleted::boolean[]) as deleted
    , unnest(@parent_contract_created_at::timestamptz[]) as created_at
    , unnest(@parent_contract_name::varchar[]) as name
    , unnest(@parent_contract_symbol::varchar[]) as symbol
    , unnest(@parent_contract_address::varchar[]) as address
    , unnest(@parent_contract_creator_address::varchar[]) as creator_address
    , unnest(@parent_contract_chain::int[]) as chain
    , unnest(@parent_contract_description::varchar[]) as description
),
-- child_contracts_data is the data to be inserted for the child contract.
child_contracts_data(id, deleted, created_at, name, address, creator_address, chain, description, parent_id) as (
  select
    unnest(@child_contract_id::varchar[]) as id
    , unnest(@child_contract_deleted::boolean[]) as deleted
    , unnest(@child_contract_created_at::timestamptz[]) as created_at
    , unnest(@child_contract_name::varchar[]) as name
    -- For child contracts, the address is the unique identifier specific to each contract
    -- that uniquely identifies a child contract within a contract.
    , unnest(@child_contract_address::varchar[]) as address
    , unnest(@child_contract_creator_address::varchar[]) as creator_address
    , unnest(@child_contract_chain::int[]) as chain
    , unnest(@child_contract_description::varchar[]) as description
     -- This field is only used as condition of the join
    , unnest(@child_contract_parent_address::varchar[]) as parent_address
),
-- tokens_data is the data to be inserted for the tokens belonging to a child contract
tokens_data(id, deleted, created_at, name, description, token_type, token_id, quantity, ownership_history, ownership_history_start_idx, ownership_history_end_idx, external_url, block_number, owner_user_id, owned_by_wallets, chain, contract, is_provider_marked_spam, last_synced) as (
  select
    unnest(@token_id::varchar[]) as id
    , unnest(@token_deleted::boolean[]) as deleted
    , unnest(@token_created_at::timestamptz[]) as created_at
    , unnest(@token_name::varchar[]) as name
    , unnest(@token_description::varchar[]) as description
    , unnest(@token_token_type::varchar[]) as token_type
    , unnest(@token_token_id::varchar[]) as token_id
    , unnest(@token_quantity::varchar[]) as quantity
    , @token_ownership_history::jsonb[] as ownership_history
    , unnest(@token_ownership_history_start_idx::int[]) as ownership_history_start_idx
    , unnest(@token_ownership_history_end_idx::int[]) as ownership_history_end_idx
    , unnest(@token_external_url::varchar[]) as external_url
    , unnest(@token_block_number::bigint[]) as block_number
    , unnest(@token_owner_user_id::varchar[]) as owner_user_id
    , @token_owned_by_wallets::varchar[] as owned_by_wallets
    , unnest(@token_owned_by_wallets_start_idx::int[]) as owned_by_wallets_start_idx
    , unnest(@token_owned_by_wallets_end_idx::int[]) as owned_by_wallets_end_idx
    , unnest(@token_chain::int[]) as chain
    , unnest(@token_is_provider_marked_spam::bool[]) as is_provider_marked_spam
    , unnest(@token_last_synced::timestamptz[]) as last_synced
     -- This field is only used as condition of the join
    , unnest(@token_contract_address::varchar[]) as contract_address
),
-- Insert parent contracts, returning the inserted or updated rows
insert_parent_contracts as (
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
  on conflict (chain, parent_id, address)
  do update set deleted = excluded.deleted
    , name = excluded.name
    , symbol = excluded.symbol
    , creator_address = excluded.creator_address
    , description = excluded.description
    , last_updated = now()
  returning *
),
-- Insert child contracts with reference to the parent contract, returning the inserted or updated rows
insert_child_contracts as (
  insert into contracts (id, deleted, created_at, name, address, creator_address, chain, description, parent_id)
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
      , parent_id
    from child_contracts_data
    -- Join on the inserted parent_contracts to get the parent's id
    join insert_parent_contracts on child_contracts_data.chain = insert_parent_contracts.chain and child_contracts_data.parent_address = insert_parent_contracts.address
  )
  on conflict (chain, parent_id, address)
  do update set deleted = excluded.deleted
    , name = excluded.name
    , creator_address = excluded.creator_address
    , description = excluded.description
    , last_updated = now()
  returning *
)
-- Finally insert the tokens, with reference to the parent and child contract ids
-- Note that media related columns (token_uri, token_metadata, media) is not inserted or updated here, because it is assumed
-- that this data will be inserted later via tokenprocessing.
-- User-scoped data (collectors_note, is_user_marked_spam) is also not updated here.
insert into tokens(id, deleted, created_at, name, description, token_type, quantity, ownership_history, external_url, block_number, owner_user_id, owned_by_wallets, chain, contract, is_provider_marked_spam, last_synced, child_contract_id) (
  select
    id
    , deleted 
    , created_at
    , last_updated
    , name
    , description
    , token_type
    , token_id
    , quantity
    , ownership_history[ownership_history_start_idx::int:ownership_history_end_idx::int]
    , external_url
    , block_number
    , owner_user_id
    , owned_by_wallets[owned_by_wallets_start_idx::int:owned_by_wallets_end_idx::int]
    , chain
    , insert_parent_contracts.id
    , is_provider_marked_spam
    , last_synced
    , insert_child_contracts.id
  from tokens_data
  -- Join on the inserted parent contracts to get the parent's id
  join insert_parent_contracts on tokens_data.chain = insert_child_contracts.chain and tokens_data.contract_address = insert_parent_contracts.address
  -- Join on the inserted child contracts to get the child's id
  join insert_child_contracts on tokens_data.chain = insert_child_contracts.chain and tokens_data.contract_address = insert_child_contracts.address
)
on conflict (token_id, contract, chain, owner_user_id) where deleted = false
do update set
  token_type = excluded.token_type
  , chain = excluded.chain
  , name = excluded.name
  , description = excluded.description
  , quantity = excluded.quantity
  , owner_user_id = excluded.owner_user_id
  , owned_by_wallets = excluded.owned_by_wallets
  , ownership_history = tokens.ownership_history || excluded.ownership_history
  , external_url = excluded.external_url
  , block_number = excluded.block_number
  , last_updated = excluded.last_updated
  , is_provider_marked_spam = excluded.is_provider_marked_spam
  , last_synced = greatest(excluded.last_synced,tokens.last_synced)
returning *;
