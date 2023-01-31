alter table pii_for_users add column if not exists pii_external_socials jsonb not null default '{}';
drop view if exists users_with_pii;
create or replace view users_with_pii as
    select users.*, pii_for_users.pii_email_address,pii_for_users.pii_external_socials from users left join pii_for_users on users.id = pii_for_users.user_id and pii_for_users.deleted = false;


-- will later be in the pii schema
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