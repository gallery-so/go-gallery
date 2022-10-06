CREATE TABLE IF NOT EXISTS notifications (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    ACTOR_ID varchar(255),
    OWNER_ID varchar(255),
    VERSION int,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ACTION varchar(255) NOT NULL,
    DATA JSONB,
    SEEN boolean NOT NULL DEFAULT false,
    AMOUNT int NOT NULL DEFAULT 1
);


CREATE INDEX IF NOT EXISTS notification_owner_id_idx ON notifications (owner_id);

ALTER TABLE users ADD COLUMN IF NOT EXISTS notification_settings JSONB;
