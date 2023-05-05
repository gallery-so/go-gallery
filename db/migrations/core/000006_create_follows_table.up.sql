/* {% require_sudo %} */
CREATE TABLE IF NOT EXISTS follows (
    ID varchar(255) PRIMARY KEY,
    FOLLOWER varchar(255) NOT NULL REFERENCES users (ID),
    FOLLOWEE varchar(255) NOT NULL REFERENCES users (ID),
    DELETED bool NOT NULL DEFAULT false,
    UNIQUE (FOLLOWER, FOLLOWEE)
);

CREATE INDEX IF NOT EXISTS follows_follower_idx ON follows (FOLLOWER);

CREATE INDEX IF NOT EXISTS follows_followee_idx ON follows (FOLLOWEE);
