/* {% require_sudo %} */
create index if not exists collections_owner_idx on collections (owner_user_id) where deleted = false;
