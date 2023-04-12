create table if not exists blockchain_statistics (
    id character varying(255) PRIMARY KEY,
    deleted boolean default false not null,
    version integer default 0 not null,
    created_at timestamp with time zone default CURRENT_TIMESTAMP not null,
    last_updated timestamp with time zone default CURRENT_TIMESTAMP not null,
    block_start bigint not null,
    block_end bigint not null,
    total_logs bigint,
    total_transfers bigint,
    total_tokens bigint,
    total_contracts bigint,
    success boolean default false not null,
    contract_stats jsonb,
    token_stats jsonb,
    processing_time_seconds bigint
);

create unique index if not exists blockchain_statistics_blocks_idx on blockchain_statistics (block_start, block_end) where deleted = false;
