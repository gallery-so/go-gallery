create table if not exists base.contracts (
    address text primary key,
    simplehash_lookup_nft_id text not null,
    last_simplehash_sync timestamptz,
    created_at timestamptz not null default now(),
    last_updated timestamptz not null default now()
);

create index if not exists base_contracts_last_simplehash_sync_null on base.contracts(last_simplehash_sync) where last_simplehash_sync is null;

create table if not exists base.collections (
    id text primary key,
    simplehash_lookup_nft_id text not null,
    last_simplehash_sync timestamptz,
    created_at timestamptz not null default now(),
    last_updated timestamptz not null default now()
);

create index if not exists base_collections_last_simplehash_sync_null on base.collections(last_simplehash_sync) where last_simplehash_sync is null;

create table if not exists base.tokens (
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

create index if not exists base_tokens_contract_address_token_id on base.tokens(contract_address, token_id);

create table if not exists zora.contracts (
    address text primary key,
    simplehash_lookup_nft_id text not null,
    last_simplehash_sync timestamptz,
    created_at timestamptz not null default now(),
    last_updated timestamptz not null default now()
);

create index if not exists zora_contracts_last_simplehash_sync_null on zora.contracts(last_simplehash_sync) where last_simplehash_sync is null;

create table if not exists zora.collections (
    id text primary key,
    simplehash_lookup_nft_id text not null,
    last_simplehash_sync timestamptz,
    created_at timestamptz not null default now(),
    last_updated timestamptz not null default now()
);

create index if not exists zora_collections_last_simplehash_sync_null on zora.collections(last_simplehash_sync) where last_simplehash_sync is null;

create table if not exists zora.tokens (
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

create index if not exists zora_tokens_contract_address_token_id on zora.tokens(contract_address, token_id);