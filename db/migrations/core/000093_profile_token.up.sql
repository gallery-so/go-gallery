create table if not exists profile_pictures (
  id varchar(255) primary key,
  deleted boolean default false not null,
  created_at timestamp with time zone default now() not null,
  last_updated timestamp with time zone default now() not null,
  source_type varchar(255) not null,
  token_id varchar(255) references tokens(id)
);
alter table users add column profile_picture_id varchar(255) references profile_pictures(id);
