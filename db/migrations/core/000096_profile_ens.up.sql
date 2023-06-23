alter table profile_images add column if not exists wallet_id varchar(255) constraint profile_images_wallet_id_fk references wallets(id);
alter table profile_images add column if not exists ens_avatar_uri varchar;
