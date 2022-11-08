create table backups
(
    id varchar(255) not null primary key,
    deleted boolean default false not null,
    version integer default 0,
    created_at timestamp with time zone default CURRENT_TIMESTAMP not null,
    last_updated timestamp with time zone default CURRENT_TIMESTAMP not null,
    gallery_id varchar(255),
    gallery jsonb
);

