-- name: UpsertContracts :many
insert into contracts
(
  id
  , deleted
  , version
  , created_at
  , last_updated
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
  , unnest(@last_updated::timestamptz[])
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
  , last_updated = excluded.last_updated
returning *;
