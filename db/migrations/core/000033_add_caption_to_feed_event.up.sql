/* {% require_sudo %} */
ALTER TABLE events ADD COLUMN IF NOT EXISTS caption VARCHAR;
ALTER TABLE feed_events ADD COLUMN IF NOT EXISTS caption VARCHAR;
