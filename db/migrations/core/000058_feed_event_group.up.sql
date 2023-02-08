alter table feed_events add column if not exists group_id varchar(255);
create unique index if not exists feed_event_group_id_idx on feed_events (group_id);