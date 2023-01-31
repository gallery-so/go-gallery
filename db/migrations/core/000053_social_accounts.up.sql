alter table users add column if not exists external_socials jsonb not null default '{}';


create table if not exists social_account_auth (
    id varchar(255) not null primary key,
    deleted boolean default false not null,
    version integer default 0,
    created_at timestamp with time zone default CURRENT_TIMESTAMP not null,
    last_updated timestamp with time zone default CURRENT_TIMESTAMP not null,
    user_id varchar(255) not null references users(id),
    provider varchar not null,
    access_token varchar,
    refresh_token varchar
);

create unique index if not exists social_account_auth_user_id_provider_idx on social_account_auth(user_id, provider);