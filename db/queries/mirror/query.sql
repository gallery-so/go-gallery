-- name: UpsertEthereumOwnerEntry :exec
insert into ethereum.owners (
    simplehash_kafka_key,
    simplehash_nft_id,
    last_updated,
    kafka_offset,
    kafka_partition,
    kafka_timestamp,
    contract_address,
    token_id,
    owner_address,
    quantity,
    collection_id,
    first_acquired_date,
    last_acquired_date,
    first_acquired_transaction,
    last_acquired_transaction,
    minted_to_this_wallet,
    airdropped_to_this_wallet,
    sold_to_this_wallet
    )
    values (
        @simplehash_kafka_key,
        @simplehash_nft_id,
        now(),
        @kafka_offset,
        @kafka_partition,
        @kafka_timestamp,
        @contract_address,
        @token_id,
        @owner_address,
        @quantity,
        @collection_id,
        @first_acquired_date,
        @last_acquired_date,
        @first_acquired_transaction,
        @last_acquired_transaction,
        @minted_to_this_wallet,
        @airdropped_to_this_wallet,
        @sold_to_this_wallet
    )
    on conflict (simplehash_kafka_key) do update
        set simplehash_nft_id = excluded.simplehash_nft_id,
            contract_address = excluded.contract_address,
            token_id = excluded.token_id,
            owner_address = excluded.owner_address,
            quantity = excluded.quantity,
            collection_id = excluded.collection_id,
            first_acquired_date = excluded.first_acquired_date,
            last_acquired_date = excluded.last_acquired_date,
            first_acquired_transaction = excluded.first_acquired_transaction,
            last_acquired_transaction = excluded.last_acquired_transaction,
            minted_to_this_wallet = excluded.minted_to_this_wallet,
            airdropped_to_this_wallet = excluded.airdropped_to_this_wallet,
            sold_to_this_wallet = excluded.sold_to_this_wallet;

-- name: DeleteEthereumOwnerEntry :exec
delete from ethereum.owners where simplehash_kafka_key = @simplehash_kafka_key;