create table if not exists communities (
    id varchar(255) primary key,
    version int not null default 0,
    community_type int not null,
    key1 text not null,
    key2 text not null,
    key3 text not null,
    key4 text not null,
    name text not null,
    override_name text,
    description text not null,
    override_description text,
    profile_image_url text,
    override_profile_image_url text,
    badge_url text,
    override_badge_url text,
    contract_id varchar(255) references contracts(id),
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    deleted boolean not null default false
);

create unique index if not exists communities_community_type_community_subtype_community_key_idx
    on communities (community_type, key1, key2, key3, key4)
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
    token_definition_id varchar(255) not null references token_definitions(id),
    community_id varchar(255) not null references communities(id),
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    deleted boolean not null default false
);

create unique index if not exists token_community_memberships_community_id_token_def_id_idx
    on token_community_memberships (community_id, token_definition_id)
    where not deleted;

create index if not exists token_community_memberships_token_def_id_idx
    on token_community_memberships (token_definition_id)
    where not deleted;

create index if not exists tokens_with_token_definition_token_definition_id_idx
    on tokens (token_definition_id)
    where not deleted;

create table if not exists community_creators (
    id varchar(255) primary key,
    version int not null default 0,
    creator_type int not null,
    community_id varchar(255) not null references communities(id),
    creator_user_id varchar(255) references users(id),
    creator_address varchar(255),
    creator_address_l1_chain int,
    creator_address_chain int,
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    deleted boolean not null default false,
    check (
            (creator_user_id is not null and creator_address is null and creator_address_l1_chain is null and creator_address_chain is null) or
            (creator_user_id is null and creator_address is not null and creator_address_l1_chain is not null and creator_address_chain is not null)
        )
);

create unique index if not exists community_creators_community_id_type_user_id_address_chain_idx
    on community_creators (community_id, creator_type, creator_user_id, creator_address, creator_address_l1_chain);

create table if not exists community_contract_providers (
    id varchar(255) primary key,
    version int not null default 0,
    contract_id varchar(255) not null references contracts(id),
    community_type int not null,
    is_valid_provider boolean not null,
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    deleted boolean not null default false
);

create unique index if not exists community_contract_providers_contract_id_community_type_idx
    on community_contract_providers (contract_id, community_type)
    where not deleted;