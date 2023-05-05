/* {% require_sudo %} */
ALTER TABLE USERS ADD COLUMN email VARCHAR(255);
ALTER TABLE USERS ADD COLUMN email_verified INT NOT NULL DEFAULT 0;
ALTER TABLE USERS ADD COLUMN email_unsubscriptions JSONB NOT NULL DEFAULT '{"all":false}';

CREATE UNIQUE INDEX IF NOT EXISTS users_email_idx ON users (email);
