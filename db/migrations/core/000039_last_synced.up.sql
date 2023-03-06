/* {% require_sudo %} */
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS last_synced TIMESTAMPTZ NOT NULL DEFAULT NOW();