alter table pii.for_users rename to pii_for_users;
alter table pii.user_view rename to users_with_pii;

alter table pii.pii_for_users set schema public;
alter table pii.users_with_pii set schema public;

drop schema if exists pii;