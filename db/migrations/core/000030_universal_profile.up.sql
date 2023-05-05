/* {% require_sudo %} */
ALTER TABLE users ADD COLUMN universal boolean NOT NULL DEFAULT false;
DROP INDEX IF EXISTS users_username_idempotent_idx;
CREATE UNIQUE INDEX users_username_idempotent_idx ON users (username_idempotent) WHERE deleted = false AND universal = false;