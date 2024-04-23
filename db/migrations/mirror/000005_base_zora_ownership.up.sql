create table base.owners (
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

create unique index if not exists base_owner_contract_token_id on base.owners(owner_address, contract_address, token_id);
create unique index if not exists base_contract_token_id_owner on base.owners(contract_address, token_id, owner_address);

create table zora.owners (
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

create unique index if not exists zora_owner_contract_token_id on zora.owners(owner_address, contract_address, token_id);
create unique index if not exists zora_contract_token_id_owner on zora.owners(contract_address, token_id, owner_address);