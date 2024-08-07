create table if not exists moshicam.nsfw_tokens (
    id varchar(255) not null primary key,
    deleted boolean default false not null,
    last_updated timestamp with time zone default current_timestamp not null,
    created_at timestamp with time zone default current_timestamp not null,
    contract_address text not null,
    token_id decimal not null,
    nsfw bool not null
);

create unique index if not exists nsfw_tokens_contract_token_idx on moshicam.nsfw_tokens(contract_address, token_id) where not deleted;


