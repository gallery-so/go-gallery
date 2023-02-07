alter table pii.for_users add column if not exists pii_external_socials jsonb not null default '{}';

drop view if exists pii.user_view;
create or replace view pii.user_view as
    select users.*, for_users.pii_email_address, for_users.pii_external_socials from users left join pii.for_users on users.id = for_users.user_id and for_users.deleted = false;


create table if not exists pii.social_account_auth (
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

grant select on pii.social_account_auth to access_ro_pii;
grant all on pii.social_account_auth to access_rw_pii;
alter table pii.social_account_auth owner to access_rw_pii;

create unique index if not exists social_account_auth_user_id_provider_idx on pii.social_account_auth(user_id, provider);