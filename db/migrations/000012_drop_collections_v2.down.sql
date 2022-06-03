CREATE TABLE IF NOT EXISTS collections_v2
(
    ID              varchar(255) PRIMARY KEY,
    DELETED         boolean     NOT NULL DEFAULT false,
    OWNER_USER_ID   varchar(255),
    NFTS            varchar(255)[],
    VERSION         int,
    LAST_UPDATED    timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CREATED_AT      timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    HIDDEN          boolean     NOT NULL DEFAULT false,
    COLLECTORS_NOTE varchar,
    NAME            varchar(255),
    LAYOUT          jsonb
);