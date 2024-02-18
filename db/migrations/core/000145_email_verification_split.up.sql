set role to access_rw_pii;

alter table pii.for_users rename column pii_email_address to pii_unverified_email_address;
alter table pii.for_users add column pii_verified_email_address text;

update pii.for_users
    set
        pii_verified_email_address = pii_unverified_email_address
    from
        users
    where
        users.id = pii.for_users.user_id
        and users.email_verified > 0;

update pii.for_users
    set
        pii_unverified_email_address = null
    from
        users
    where
        users.id = pii.for_users.user_id
        and users.email_verified > 0;

drop index if exists pii.pii_for_users_pii_email_address_idx;

create unique index if not exists pii_for_users_pii_verified_email_address_idx on pii.for_users (pii_verified_email_address);

set role to access_rw;

alter table users drop column if exists email_verified cascade;

set role to access_rw_pii;

drop view if exists pii.user_view;
create or replace view pii.user_view as
    select users.*,
        for_users.pii_unverified_email_address,
        for_users.pii_verified_email_address,
        for_users.pii_socials
    from users
        left join pii.for_users
            on users.id = for_users.user_id
                and for_users.deleted = false;

set role to access_rw;