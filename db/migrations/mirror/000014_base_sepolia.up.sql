create table base_sepolia.owners (
    simplehash_kafka_key text primary key,
    simplehash_nft_id text,
    contract_address text,
    token_id decimal,
    owner_address text,
    quantity decimal,
    collection_id text,
    first_acquired_date timestamptz,
    last_acquired_date timestamptz,
    first_acquired_transaction text,
    last_acquired_transaction text,
    minted_to_this_wallet boolean,
    airdropped_to_this_wallet boolean,
    sold_to_this_wallet boolean,
    created_at timestamptz not null default now(),
    last_updated timestamptz not null default now(),
    kafka_offset bigint,
    kafka_partition int,
    kafka_timestamp timestamptz
);

create unique index if not exists base_sepolia_owner_contract_token_id on base_sepolia.owners(owner_address, contract_address, token_id);
create unique index if not exists base_sepolia_contract_token_id_owner on base_sepolia.owners(contract_address, token_id, owner_address);

create table if not exists base_sepolia.contracts (
    address text primary key,
    simplehash_lookup_nft_id text not null,
    last_simplehash_sync timestamptz,
    created_at timestamptz not null default now(),
    last_updated timestamptz not null default now(),
    type text,
    name text,
    symbol text,
    deployed_by text,
    deployed_via_contract text,
    owned_by text,
    has_multiple_collections boolean
);

create index if not exists base_sepolia_contracts_last_simplehash_sync_null on base_sepolia.contracts(last_simplehash_sync) where last_simplehash_sync is null;

create table if not exists base_sepolia.tokens (
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

create index if not exists base_sepolia_tokens_contract_address_token_id on base_sepolia.tokens(contract_address, token_id);

create index if not exists base_sepolia_tokens_collection_id on base_sepolia.tokens (collection_id);

