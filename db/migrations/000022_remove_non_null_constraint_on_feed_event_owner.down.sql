-- This downgrade will fail if there are already existing null values.
ALTER TABLE feed_events ALTER COLUMN owner_id SET NOT NULL;