drop index if exists comments_created_at_id_feed_event_id_idx;
CREATE UNIQUE INDEX IF NOT EXISTS comments_created_at_id_feed_event_id_idx ON comments (feed_event_id, created_at desc, id desc);
drop index if exists comments_created_at_id_post_id_idx;
CREATE UNIQUE INDEX IF NOT EXISTS comments_created_at_id_post_id_idx ON comments (post_id, created_at desc, id desc);

alter table comments add column removed bool default false not null;


