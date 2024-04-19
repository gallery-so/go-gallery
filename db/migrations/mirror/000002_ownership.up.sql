create table ethereum.owners (
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
    sold_to_this_wallet boolean
);