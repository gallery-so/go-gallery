CREATE TABLE IF NOT EXISTS early_access
(
    address varchar(255) NOT NULL PRIMARY KEY
);

CREATE UNIQUE INDEX IF NOT EXISTS lowercase_address ON early_access (lower(address));
