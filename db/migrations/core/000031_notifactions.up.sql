/* {% require_sudo %} */
CREATE TABLE IF NOT EXISTS notifications (
    ID varchar(255) PRIMARY KEY,
    DELETED boolean NOT NULL DEFAULT false,
    OWNER_ID varchar(255),
    VERSION int DEFAULT 0,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ACTION varchar(255) NOT NULL,
    DATA JSONB,
    EVENT_IDS varchar(255)[],
    FEED_EVENT_ID varchar(255) REFERENCES feed_events (ID),
    COMMENT_ID varchar(255) REFERENCES comments (ID),
    GALLERY_ID varchar(255) REFERENCES galleries (ID),
    SEEN boolean NOT NULL DEFAULT false,
    AMOUNT int NOT NULL DEFAULT 1
);


CREATE INDEX IF NOT EXISTS notification_owner_id_idx ON notifications (owner_id);

CREATE INDEX IF NOT EXISTS notification_created_at_id_idx ON notifications (created_at, id);

ALTER TABLE users ADD COLUMN IF NOT EXISTS notification_settings JSONB;

ALTER TABLE events ADD COLUMN IF NOT EXISTS gallery_id varchar(255) REFERENCES galleries (id);
ALTER TABLE events ADD COLUMN IF NOT EXISTS comment_id varchar(255) REFERENCES comments (id);
ALTER TABLE events ADD COLUMN IF NOT EXISTS admire_id varchar(255) REFERENCES admires (id);
ALTER TABLE events ADD COLUMN IF NOT EXISTS feed_event_id varchar(255) REFERENCES feed_events (id);
ALTER TABLE events ADD COLUMN IF NOT EXISTS external_id varchar(255);
