create table if not exists external_social_connections (
    id varchar(255) primary key,
    version int not null default 0,
    social_account_type varchar(255) not null,
    follower_id varchar(255) references users(id) not null,
    followee_id varchar(255) references users(id) not null,
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    deleted boolean not null default false
);

create index if not exists external_social_connections_social_account_type_follower_id_idx on external_social_connections (social_account_type, follower_id) where (deleted = false);