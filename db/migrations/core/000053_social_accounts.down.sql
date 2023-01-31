alter table users drop column if exists external_socials;

drop table if exists social_account_auth;
drop index if exists social_account_auth_user_id_provider_idx;