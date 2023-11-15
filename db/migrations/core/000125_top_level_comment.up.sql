alter table comments add column top_level_comment_id varchar(255) references comments(id);

create index top_level_comment_id_idx on comments(top_level_comment_id);