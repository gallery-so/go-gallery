/* {% require_sudo %} */
ALTER TABLE contracts ADD COLUMN IF NOT EXISTS CREATOR_ADDRESS varchar(255);