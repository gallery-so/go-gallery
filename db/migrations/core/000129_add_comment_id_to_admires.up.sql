alter table admires add column if not exists comment_id varchar(255) references comments(id);
create unique index admire_actor_comment_idx on admires(actor_id, comment_id) where not deleted;
