alter table feed_blocklist drop column action;
alter table feed_blocklist add column reason varchar;
alter table feed_blocklist add column active bool default true;
create unique index feed_blocklist_user_id_idx on feed_blocklist(user_id) where not deleted;
