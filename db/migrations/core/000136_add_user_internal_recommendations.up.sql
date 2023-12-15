create table if not exists user_internal_recommendations (
  id character varying(255) primary key,
  user_id character varying(255) references users (id),
  created_at timestamp with time zone default current_timestamp not null,
  last_updated timestamp with time zone default current_timestamp not null,
  deleted boolean default false not null
);
create unique index user_internal_recommendations_user_id_idx on user_internal_recommendations(user_id) where not deleted;
