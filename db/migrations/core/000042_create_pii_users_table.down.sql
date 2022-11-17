drop view if exists users_with_pii;
drop table if exists pii_users;
drop table if exists dev_metadata_users;
drop index if exists pii_users_pii_email_address_idx;