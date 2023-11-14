alter table comments add column reply_ancestors varchar(255)[];

create index comments_reply_ancestors_idx on comments using gin(reply_ancestors);