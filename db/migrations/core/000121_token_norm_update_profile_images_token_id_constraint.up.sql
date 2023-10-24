alter table profile_images drop constraint profile_images_token_id_fkey;
alter table profile_images add constraint profile_images_token_id_fkey foreign key(token_id) references tokens(id);
