/* {% require_sudo %} */
CREATE UNIQUE INDEX users_username_idempotent_idx ON users (username_idempotent) WHERE deleted = false;