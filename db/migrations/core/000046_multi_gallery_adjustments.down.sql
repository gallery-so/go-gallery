alter table galleries drop constraint if exists exposition_cst;
create unique index if not exists position_idx on galleries (owner_user_id, position) where deleted = false;