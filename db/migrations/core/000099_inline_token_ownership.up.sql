create index if not exists contracts_override_creator_user_id_idx on contracts (override_creator_user_id) where deleted = false;
create index if not exists contracts_owner_address_creator_address_coalesce_idx on contracts (coalesce(nullif(owner_address, ''), nullif(creator_address, ''))) where deleted = false;

alter table tokens add column if not exists is_creator_token boolean not null default false;

-- Note: this takes a while. (51s on local Docker instance)
alter table tokens add column if not exists is_holder_token boolean not null
    generated always as (cardinality(owned_by_wallets) > 0)
    stored;

create index if not exists tokens_is_creator_token_idx on tokens (is_creator_token) where deleted = false;
create index if not exists tokens_is_holder_token_idx on tokens (is_holder_token) where deleted = false;