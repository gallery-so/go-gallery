alter table admires drop constraint admires_token_id_fkey;
alter table admires add constraint admires_token_id_fkey foreign key(token_id) references tokens(id);
