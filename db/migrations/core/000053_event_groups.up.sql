alter table events add column if not exists group_id varchar(255);
create index if not exists group_id_idx on events (group_id);