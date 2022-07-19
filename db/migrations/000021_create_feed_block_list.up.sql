CREATE TABLE IF NOT EXISTS feed_blocklist (
    ID varchar(255) PRIMARY KEY,
    USER_ID varchar(255) REFERENCES users (id),
    ACTION varchar(255) NOT NULL,
    LAST_UPDATED timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DELETED boolean NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX feed_blocklist_user_id_action_idx ON feed_blocklist (user_id, action);
