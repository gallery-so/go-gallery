-- name: UpsertTokens :many
insert into tokens
(
  id
  , deleted
  , version
  , created_at
  , last_updated
  , name
  , description
  , collectors_note
  , token_type
  , token_id
  , quantity
  , ownership_history
  , media
  , token_media
  , fallback_media
  , token_metadata
  , external_url
  , block_number
  , owner_user_id
  , owned_by_wallets
  , chain
  , contract
  , is_user_marked_spam
  , is_provider_marked_spam
  , last_synced
  , token_uri
) (
  select
    id
    , deleted 
    , version
    , created_at
    , last_updated
    , name
    , description
    , collectors_note
    , token_type
    , token_id
    , quantity
    , ownership_history[ownership_history_start_idx::int:ownership_history_end_idx::int]
    , media
    , token_media
    , fallback_media
    , token_metadata
    , external_url
    , block_number
    , owner_user_id
    , owned_by_wallets[owned_by_wallets_start_idx::int:owned_by_wallets_end_idx::int]
    , chain
    , contract
    , is_user_marked_spam
    , is_provider_marked_spam
    , last_synced
    , token_uri
  from (
    select
      unnest(@id::varchar[]) as id
      , unnest(@deleted::boolean[]) as deleted
      , unnest(@version::int[]) as version
      , unnest(@created_at::timestamptz[]) as created_at
      , unnest(@last_updated::timestamptz[]) as last_updated
      , unnest(@name::varchar[]) as name
      , unnest(@description::varchar[]) as description
      , unnest(@collectors_note::varchar[]) as collectors_note
      , unnest(@token_type::varchar[]) as token_type
      , unnest(@token_id::varchar[]) as token_id
      , unnest(@quantity::varchar[]) as quantity
      , @ownership_history::jsonb[] as ownership_history
      , unnest(@ownership_history_start_idx::int[]) as ownership_history_start_idx
      , unnest(@ownership_history_end_idx::int[]) as ownership_history_end_idx
      , unnest(@media::jsonb[]) as media
      , unnest(@token_media::varchar[]) as token_media
      , unnest(@fallback_media::jsonb[]) as fallback_media
      , unnest(@token_metadata::jsonb[]) as token_metadata
      , unnest(@external_url::varchar[]) as external_url
      , unnest(@block_number::bigint[]) as block_number
      , unnest(@owner_user_id::varchar[]) as owner_user_id
      , @owned_by_wallets::varchar[] as owned_by_wallets
      , unnest(@owned_by_wallets_start_idx::int[]) as owned_by_wallets_start_idx
      , unnest(@owned_by_wallets_end_idx::int[]) as owned_by_wallets_end_idx
      , unnest(@chain::int[]) as chain
      , unnest(@contract::varchar[]) as contract
      , unnest(@is_user_marked_spam::bool[]) as is_user_marked_spam
      , unnest(@is_provider_marked_spam::bool[]) as is_provider_marked_spam
      , unnest(@last_synced::timestamptz[]) as last_synced
      , unnest(@token_uri::varchar[]) as token_uri
  ) bulk_upsert
)
on conflict (token_id, contract, chain, owner_user_id) where deleted = false
do update set
  token_type = excluded.token_type
  , chain = excluded.chain
  , name = excluded.name
  , description = excluded.description
  , token_uri = excluded.token_uri
  , quantity = excluded.quantity
  , owner_user_id = excluded.owner_user_id
  , owned_by_wallets = excluded.owned_by_wallets
  , ownership_history = tokens.ownership_history || excluded.ownership_history
  , fallback_media = excluded.fallback_media
  , token_metadata = excluded.token_metadata
  , external_url = excluded.external_url
  , block_number = excluded.block_number
  , version = excluded.version
  , last_updated = excluded.last_updated
  , is_user_marked_spam = tokens.is_user_marked_spam
  , is_provider_marked_spam = excluded.is_provider_marked_spam
  , last_synced = greatest(excluded.last_synced,tokens.last_synced)
returning *;
