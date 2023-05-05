/* {% require_sudo %} */
ALTER TABLE collections ADD COLUMN IF NOT EXISTS token_settings jsonb;