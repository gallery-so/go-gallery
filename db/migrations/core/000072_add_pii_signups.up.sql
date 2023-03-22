create table if not exists pii.signups (
    user_id varchar(255) primary key references users(id),
    ip_address text not null,
    created_at timestamptz not null
);

