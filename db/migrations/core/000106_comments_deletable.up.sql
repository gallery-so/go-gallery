drop index if exists comments_created_at_id_feed_event_id_idx;
CREATE UNIQUE INDEX IF NOT EXISTS comments_created_at_id_feed_event_id_idx ON comments (feed_event_id, created_at desc, id desc);
drop index if exists comments_created_at_id_post_id_idx;
CREATE UNIQUE INDEX IF NOT EXISTS comments_created_at_id_post_id_idx ON comments (post_id, created_at desc, id desc);

alter table comments add column removed bool default false not null;

create table if not exists mentions (
  id varchar(255) not null primary key,
  post_id varchar(255) references posts(id),
  comment_id varchar(255) references comments(id),
  user_id varchar(255) references users(id),
  contract_id varchar(255) references contracts(id),
  start int,
  length int,
  created_at timestamp not null default now(),
  deleted bool default false not null
);

alter table events add column mention_id varchar(255) references mentions(id);
alter table notifications add column mention_id varchar(255) references mentions(id);


