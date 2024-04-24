-- name: ProcessEthereumOwnerEntry :batchexec
with deletion as (
    delete from ethereum.owners where @should_delete::bool and simplehash_kafka_key = @simplehash_kafka_key
)
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
    select
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
    where @should_upsert::bool
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

-- name: ProcessBaseOwnerEntry :batchexec
with deletion as (
    delete from base.owners where @should_delete::bool and simplehash_kafka_key = @simplehash_kafka_key
)
insert into base.owners (
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
    select
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
    where @should_upsert::bool
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

-- name: ProcessZoraOwnerEntry :batchexec
with deletion as (
    delete from zora.owners where @should_delete::bool and simplehash_kafka_key = @simplehash_kafka_key
)
insert into zora.owners (
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
    select
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
    where @should_upsert::bool
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


-- name: ProcessEthereumTokenEntry :batchexec
with deletion as (
    delete from ethereum.tokens where @should_delete::bool and simplehash_kafka_key = @simplehash_kafka_key
),

contract_insert as (
    insert into ethereum.contracts (address, simplehash_lookup_nft_id)
    select @contract_address, @simplehash_nft_id
    where @should_upsert::bool
    on conflict (address) do nothing
),
    
collection_insert as (
    insert into ethereum.collections (id, simplehash_lookup_nft_id)
    select @collection_id, @simplehash_nft_id
    where @should_upsert::bool
    on conflict (id) do nothing
)

insert into ethereum.tokens (
    simplehash_kafka_key,
    simplehash_nft_id,
    contract_address,
    token_id,
    name,
    description,
    previews,
    image_url,
    video_url,
    audio_url,
    model_url,
    other_url,
    background_color,
    external_url,
    on_chain_created_date,
    status,
    token_count,
    owner_count,
    contract,
    collection_id,
    last_sale,
    first_created,
    rarity,
    extra_metadata,
    image_properties,
    video_properties,
    audio_properties,
    model_properties,
    other_properties,
    last_updated,
    kafka_offset,
    kafka_partition,
    kafka_timestamp
    )
    select
        @simplehash_kafka_key,
        @simplehash_nft_id,
        @contract_address,
        @token_id,
        @name,
        @description,
        @previews,
        @image_url,
        @video_url,
        @audio_url,
        @model_url,
        @other_url,
        @background_color,
        @external_url,
        @on_chain_created_date,
        @status,
        @token_count,
        @owner_count,
        @contract,
        @collection_id,
        @last_sale,
        @first_created,
        @rarity,
        @extra_metadata,
        @image_properties,
        @video_properties,
        @audio_properties,
        @model_properties,
        @other_properties,
        now(),
        @kafka_offset,
        @kafka_partition,
        @kafka_timestamp
    where @should_upsert::bool
    on conflict (simplehash_kafka_key) do update
        set simplehash_nft_id = excluded.simplehash_nft_id,
            contract_address = excluded.contract_address,
            token_id = excluded.token_id,
            name = excluded.name,
            description = excluded.description,
            previews = excluded.previews,
            image_url = excluded.image_url,
            video_url = excluded.video_url,
            audio_url = excluded.audio_url,
            model_url = excluded.model_url,
            other_url = excluded.other_url,
            background_color = excluded.background_color,
            external_url = excluded.external_url,
            on_chain_created_date = excluded.on_chain_created_date,
            status = excluded.status,
            token_count = excluded.token_count,
            owner_count = excluded.owner_count,
            contract = excluded.contract,
            collection_id = excluded.collection_id,
            last_sale = excluded.last_sale,
            first_created = excluded.first_created,
            rarity = excluded.rarity,
            extra_metadata = excluded.extra_metadata,
            image_properties = excluded.image_properties,
            video_properties = excluded.video_properties,
            audio_properties = excluded.audio_properties,
            model_properties = excluded.model_properties,
            other_properties = excluded.other_properties,
            last_updated = now(),
            kafka_offset = excluded.kafka_offset,
            kafka_partition = excluded.kafka_partition,
            kafka_timestamp = excluded.kafka_timestamp;