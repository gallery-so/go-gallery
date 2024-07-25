create table if not exists moshicam.token_tags (
    id varchar(255) not null primary key,
    deleted boolean default false not null,
    last_updated timestamp with time zone default current_timestamp not null,
    created_at timestamp with time zone default current_timestamp not null,
    contract_address text not null,
    token_id decimal not null,
    tag_name text not null
);
create unique index if not exists token_tags_contract_token_tag_idx on moshicam.token_tags(tag_name, contract_address, token_id) where not deleted;
create index if not exists token_tags_contract_token_idx on moshicam.token_tags(contract_address, token_id) where not deleted;
