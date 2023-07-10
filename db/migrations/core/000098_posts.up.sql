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

alter table admires add column post_id varchar(255) references posts(id);
alter table comments add column post_id varchar(255) references posts(id);