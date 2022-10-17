ALTER TABLE users DROP COLUMN universal;
DROP INDEX IF EXISTS users_username_idempotent_idx;
CREATE UNIQUE INDEX users_username_idempotent_idx ON users (username_idempotent) WHERE deleted = false;