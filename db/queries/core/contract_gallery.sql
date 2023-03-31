-- name: UpsertContracts :many
insert into contracts
(
  id
  , deleted
  , version
  , created_at
  , address
  , symbol
  , name
  , creator_address
  , chain
) (
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
)
on conflict (address, chain) where deleted = false
do update set
  symbol = excluded.symbol
  , version = excluded.version
  , name = excluded.name
  , creator_address = excluded.creator_address
  , chain = excluded.chain
  , last_updated = now()
returning *;

-- name: UpsertCreatedTokens :many
with contract_subgroups_data(
  id
  , deleted
  , created_at
  , creator_id
  , parent_id
  , external_id
  , contract_address
  , chain
) as (
  select
  unnest(@contract_id::varchar[])
  , unnest(@contract_deleted::boolean[])
  , unnest(@contract_created_at::timestamptz[])
  , unnest(@contract_creator_id::varchar[])
  , unnest(@contract_parent_id::varchar[])
  , unnest(@contract_external_id::varchar[])
  , unnest(@contract_contract_address::varchar[])
  , unnest(@contract_chain::varchar[])
),
token_subgroups_data(
  id
  , deleted
  , token_id
  , subgroup_id
  , created_at
  , contract_address
  , chain
) as (
  select
  unnest(@token_id::varchar[])
  , unnest(@token_deleted::boolean[])
  , unnest(@token_token_id::varchar[])
  , unnest(@token_subgroup_id::varchar[])
  , unnest(@token_created_at::timestamptz[])
  , unnest(@token_contract_address::varchar[])
  , unnest(@token_chain::varchar[])
),
insert_contract_subgroups as (
  insert into contract_subgroups
  (
    id
    , creator_id
    , parent_id
    , external_id
    , created_at
    , deleted
  ) (
    select
    id
    , creator_id
    , parent_id
    , external_id
    , created_at
    , deleted
    from contract_subgroups_data
  )
  on conflict (creator_id, parent_id) where deleted = false
  do update set external_id = excluded.external_id , last_updated = now()
  returning *
)
insert into token_subgroups (
  id
  , token_id
  , subgroup_id
  , created_at
  , deleted
) (
  select
  t.id
  , t.token_id
  , i.id
  , t.created_at
  , t.deleted
  from token_subgroups_data t, contract_subgroups_data c, insert_contract_subgroups i
  where
    t.contract_address = c.contract_address
    and t.chain = c.chain
    and c.contract_address = i.parent_id
    and c.creator_id = i.creator_id
)
on conflict(token_id, subgroup_id) where deleted = false
do update set last_updated = now()
returning *;
