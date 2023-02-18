ALTER TABLE users ADD COLUMN push_notification_tokens varchar(256)[] NOT NULL DEFAULT '{}';
