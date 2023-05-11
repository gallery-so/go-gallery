create table if not exists token_processing_jobs (
    id varchar(255) primary key,
    created_at timestamptz not null default current_timestamp,
    token_properties jsonb not null,
    pipeline_metadata jsonb not null,
    processing_cause varchar not null,
    processor_version varchar not null,
    deleted bool not null default false
);

create table if not exists token_medias (
    id varchar(255) primary key,
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    version int not null default 0,
    contract_id varchar(255) not null references contracts(id),
    token_id varchar not null,
    chain int not null,
    active bool not null,
    metadata jsonb not null,
    media jsonb not null,
    name varchar not null,
    description varchar not null,
    processing_job_id varchar(255) not null references token_processing_jobs(id),
    deleted bool not null default false
);

create table if not exists reprocess_jobs (
    id int primary key,
    token_start_id varchar(255) not null,
    token_end_id varchar(255) not null
);

create unique index if not exists token_media_contract_token_id_chain_idx on token_medias (contract_id, token_id, chain) where active = true and deleted = false;

alter table tokens add column if not exists token_media_id varchar(255) references token_medias(id);

alter table tokens add constraint fk_tokens_contracts foreign key (contract) references contracts (id) on delete cascade on update cascade;

create index if not exists token_last_updated_idx on tokens (last_updated) where deleted = false;