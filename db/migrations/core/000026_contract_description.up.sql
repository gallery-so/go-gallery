/* {% require_sudo %} */
ALTER TABLE contracts ADD COLUMN IF NOT EXISTS description varchar;