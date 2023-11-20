create table if not exists user_blocklist (
  id character varying(255) primary key,
  created_at timestamp with time zone not null default current_timestamp,
  last_updated timestamp with time zone not null default current_timestamp,
  deleted boolean not null default false,
  user_id character varying(255) references users(id),
  blocked_user_id character varying(255) references posts(id),
  active bool default true
);
create unique index user_blocklist_user_id_blocked_user_id_idx on user_blocklist(user_id, blocked_user_id) where not deleted and active;
