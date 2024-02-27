create table privy_users (
    id varchar(255) primary key,
    privy_did varchar(255) not null,
    user_id varchar(255) references users(id),
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    deleted boolean not null default false
);

create unique index privy_users_privy_did_idx on privy_users (privy_did) where deleted = false;
create unique index privy_users_user_id_idx on privy_users (user_id) where deleted = false;