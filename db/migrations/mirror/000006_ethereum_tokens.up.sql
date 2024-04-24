create table if not exists ethereum.contracts (
    address text primary key,
    simplehash_lookup_nft_id text not null,
    last_simplehash_sync timestamptz,
    created_at timestamptz not null default now(),
    last_updated timestamptz not null default now()
);

create index if not exists ethereum_contracts_last_simplehash_sync_null on ethereum.contracts(last_simplehash_sync) where last_simplehash_sync is null;

create table if not exists ethereum.collections (
    id text primary key,
    simplehash_lookup_nft_id text not null,
    last_simplehash_sync timestamptz,
    created_at timestamptz not null default now(),
    last_updated timestamptz not null default now()
);

create index if not exists ethereum_collections_last_simplehash_sync_null on ethereum.collections(last_simplehash_sync) where last_simplehash_sync is null;

create table if not exists ethereum.tokens (
    simplehash_kafka_key text primary key,
    simplehash_nft_id text,
    contract_address text,
    token_id decimal,
    name text,
    description text,
    previews jsonb,
    image_url text,
    video_url text,
    audio_url text,
    model_url text,
    other_url text,
    background_color text,
    external_url text,
    on_chain_created_date timestamptz,
    status text,
    token_count decimal,
    owner_count decimal,
    contract jsonb,
    collection_id text,
    last_sale jsonb,
    first_created jsonb,
    rarity jsonb,
    extra_metadata text,
    image_properties jsonb,
    video_properties jsonb,
    audio_properties jsonb,
    model_properties jsonb,
    other_properties jsonb,
    created_at timestamptz not null default now(),
    last_updated timestamptz not null default now(),
    kafka_offset bigint,
    kafka_partition int,
    kafka_timestamp timestamptz
);

create index if not exists ethereum_tokens_contract_address_token_id on ethereum.tokens(contract_address, token_id);