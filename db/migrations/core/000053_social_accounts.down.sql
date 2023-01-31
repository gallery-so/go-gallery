drop view if exists users_with_pii;
create or replace view users_with_pii as
    select users.*, pii_for_users.pii_email_address from users left join pii_for_users on users.id = pii_for_users.user_id and pii_for_users.deleted = false;
alter table pii_for_users drop column if exists external_socials;



drop table if exists social_account_auth;
drop index if exists social_account_auth_user_id_provider_idx;