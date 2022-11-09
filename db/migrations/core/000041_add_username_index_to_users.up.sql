create index users_username_idempotent_id_idx on users (username_idempotent asc, id asc);
create index users_roles_idx on users using gin(roles) where deleted = false;
