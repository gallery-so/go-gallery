CREATE UNIQUE INDEX IF NOT EXISTS admires_created_at_id_feed_event_id_idx ON admires (created_at desc, id desc, feed_event_id desc);
CREATE UNIQUE INDEX IF NOT EXISTS comments_created_at_id_feed_event_id_idx ON comments (created_at desc, id desc, feed_event_id desc);
