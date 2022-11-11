create table if not exists user_roles (
    id varchar(255) primary key,
    user_id varchar(255) not null references users (id),
    role varchar(64) not null,
    version int not null default 0,
    deleted boolean not null default false,
    created_at timestamptz not null default current_timestamp,
    last_updated  timestamptz not null default current_timestamp,
    unique (user_id, role)
);
create index if not exists user_roles_role_idx on user_roles (role) where deleted = false;