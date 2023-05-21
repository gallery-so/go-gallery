create table if not exists sessions (
    id varchar(255) primary key,
    user_id varchar(255) not null references users(id),
    created_at timestamptz not null,
    created_with_user_agent text not null,
    created_with_platform text not null,
    created_with_os text not null,
    last_refreshed timestamptz not null,
    last_user_agent text not null,
    last_platform text not null,
    last_os text not null,
    current_refresh_id varchar(255) not null,
    active_until timestamptz not null,
    invalidated bool not null,
    last_updated timestamptz not null,
    deleted bool not null
);

create index if not exists sessions_user_id_idx on sessions (user_id) where deleted = false;
create unique index if not exists sessions_id_idx on sessions (id) where deleted = false;