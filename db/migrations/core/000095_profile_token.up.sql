create table if not exists profile_images (
  id varchar(255) primary key,
  user_id varchar(255) references users(id),
  token_id varchar(255) references tokens(id),
  source_type varchar(255) not null,
  deleted boolean default false not null,
  created_at timestamp with time zone default now() not null,
  last_updated timestamp with time zone default now() not null
);
create unique index if not exists profile_images_user_id_idx on profile_images(user_id);
alter table users add column if not exists profile_image_id varchar(255) references profile_images(id);
