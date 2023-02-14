set role to access_rw;

create table if not exists recommendation_results (
  id character varying(255) primary key,
  version int default 0,
  user_id character varying(255) references users (id),
  recommended_user_id character varying(255) references users (id),
  recommended_count integer,
  created_at timestamp with time zone default current_timestamp not null,
  last_updated timestamp with time zone default current_timestamp not null,
  deleted boolean default false not null,
  unique (user_id, recommended_user_id, version),
  constraint user_id_not_equal_recommend_id_constraint check (user_id != recommended_user_id)
);

drop materialized view if exists top_recommended_users;

create materialized view top_recommended_users as (
  select recommended_user_id, count(distinct user_id) frequency, now() last_updated
  from recommendation_results
  where version = 0 and deleted = false and last_updated >= now() - interval '30 days'
  group by recommended_user_id
  order by frequency desc, last_updated desc
  limit 100
);
