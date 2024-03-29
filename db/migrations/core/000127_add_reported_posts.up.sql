create table if not exists reported_posts (
  id character varying(255) primary key,
  created_at timestamp with time zone not null default current_timestamp,
  last_updated timestamp with time zone not null default current_timestamp,
  deleted boolean not null default false,
  reporter_id character varying(255) references users(id),
  post_id character varying(255) references posts(id),
  reason character varying
);
create unique index reported_posts_post_id_reported_id_reason_idx on reported_posts(post_id, reporter_id, reason) where not deleted;
