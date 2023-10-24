alter table notifications drop constraint notifications_token_id_fkey;
alter table notifications add constraint notifications_token_id_fkey foreign key(token_id) references tokens(id);
