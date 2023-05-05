/* {% require_sudo %} */
create index if not exists events_gallery_edit_idx on events (created_at, actor_id) where action in ('CollectionCreated', 'CollectorsNoteAddedToCollection', 'CollectorsNoteAddedToToken', 'TokensAddedToCollection');
