create table if not exists public.collections (
    id text primary key,
    simplehash_lookup_nft_id text not null,
    last_simplehash_sync timestamptz,
    created_at timestamptz not null default now(),
    last_updated timestamptz not null default now()
);

create index if not exists collections_last_simplehash_sync_null on public.collections(last_simplehash_sync) where last_simplehash_sync is null;