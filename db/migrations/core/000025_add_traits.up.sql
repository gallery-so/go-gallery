/* {% require_sudo %} */
ALTER TABLE users ADD COLUMN IF NOT EXISTS traits jsonb;