-- TODO: Only for testing
set role to access_rw;

create table if not exists communities (
    id varchar(255) primary key,
    version int not null default 0,
    name text not null,
    description text not null,
    community_type int not null,
    community_subtype text not null,
    community_key text not null,
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    deleted boolean not null default false
);

create unique index if not exists communities_community_type_community_subtype_community_key_idx
    on communities (community_type, community_subtype, community_key)
    where not deleted;

create table if not exists contract_community_memberships (
    id varchar(255) primary key,
    version int not null default 0,
    contract_id varchar(255) not null references contracts(id),
    community_id varchar(255) not null references communities(id),
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    deleted boolean not null default false
);

create unique index if not exists contract_community_memberships_community_id_contract_id_idx
    on contract_community_memberships (community_id, contract_id)
    where not deleted;

create index if not exists contract_community_memberships_contract_id_idx
    on contract_community_memberships (contract_id)
    where not deleted;

create table if not exists token_community_memberships (
    id varchar(255) primary key,
    version int not null default 0,
    token_id varchar(255) not null references tokens(id),
    community_id varchar(255) not null references communities(id),
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    deleted boolean not null default false
);

create unique index if not exists token_community_memberships_community_id_token_id_idx
    on token_community_memberships (community_id, token_id)
    where not deleted;

create index if not exists token_community_memberships_token_id_idx
    on token_community_memberships (token_id)
    where not deleted;
