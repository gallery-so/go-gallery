create table if not exists pii_for_users
(
    user_id       varchar(255) primary key references users (id),
    pii_email_address varchar,
    deleted       bool not null default false
);

create unique index if not exists pii_for_users_pii_email_address_idx on pii_for_users (pii_email_address) where deleted = false;

create table if not exists dev_metadata_users
(
    user_id           varchar(255) primary key references users (id),
    has_email_address bool,
    deleted           bool not null default false
);

alter table users drop column if exists email;

create view users_with_pii as
    select users.*, pii_for_users.pii_email_address from users left join pii_for_users on users.id = pii_for_users.user_id and pii_for_users.deleted = false;

drop index if exists users_email_idx;