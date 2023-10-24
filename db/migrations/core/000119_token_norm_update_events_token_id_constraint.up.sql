alter table events drop constraint events_token_id_fkey;
alter table events add constraint events_token_id_fkey foreign key(token_id) references tokens(id);
