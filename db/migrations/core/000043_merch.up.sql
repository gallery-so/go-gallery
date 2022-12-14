create table if not exists merch 
(
    id varchar(255) not null primary key,
    deleted boolean default false not null,
    version integer default 0,
    created_at timestamp with time zone default CURRENT_TIMESTAMP not null,
    last_updated timestamp with time zone default CURRENT_TIMESTAMP not null,
    token_id varchar(255),
    object_type int not null default 0,
    discount_code varchar(255),
    redeemed boolean default false not null
);

create unique index if not exists merch_token_id_idx on merch (token_id) where deleted = false;