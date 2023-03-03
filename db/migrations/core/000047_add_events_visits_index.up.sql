/* {% require_sudo %} */
create index if not exists events_visits_created_at_idx on events (created_at) where action = 'ViewedGallery';
