create table if not exists legacy_views (
  user_id varchar(255) references users(id),
  view_count int,
  last_updated timestamptz not null default current_timestamp,
  created_at timestamptz not null default current_timestamp,
  deleted boolean default false
);
