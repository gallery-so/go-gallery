create table if not exists posts (
    id varchar(255) primary key,
    version int not null default 0,
    token_ids varchar(255)[],
    actor_id varchar(255) references users(id) not null,
    caption varchar,
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    deleted boolean not null default false
);

alter table admires add column post_id varchar(255) references posts(id) not null;
alter table comments add column post_id varchar(255) references posts(id) not null;

create type feed_entity AS (
    id varchar(255),
    token_ids varchar(255)[],
    caption varchar,
    event_time timestamptz,
    source varchar,
    version int,
    owner_id varchar(255),
    group_id varchar(255),
    action varchar(255),
    data jsonb,
    event_ids varchar(255)[],
    deleted boolean,
    last_updated timestamptz,
    created_at timestamptz
);