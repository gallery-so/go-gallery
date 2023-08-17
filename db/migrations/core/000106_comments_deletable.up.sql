drop index if exists comments_created_at_id_feed_event_id_idx;
CREATE UNIQUE INDEX IF NOT EXISTS comments_created_at_id_feed_event_id_idx ON comments (created_at desc, id desc, feed_event_id);
drop index if exists comments_created_at_id_post_id_idx;
CREATE UNIQUE INDEX IF NOT EXISTS comments_created_at_id_post_id_idx ON comments (created_at desc, id desc, post_id);


