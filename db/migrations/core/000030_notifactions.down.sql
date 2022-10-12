DROP INDEX IF EXISTS notification_owner_id_idx;
DROP TABLE IF EXISTS notifications;

ALTER TABLE users DROP COLUMN IF EXISTS notification_settings;
ALTER TABLE galleries DROP COLUMN IF EXISTS views;