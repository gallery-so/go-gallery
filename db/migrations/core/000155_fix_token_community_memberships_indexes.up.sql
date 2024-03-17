drop index if exists token_community_memberships_token_def_id_idx;
create unique index token_community_memberships_token_definition_community_idx
    on token_community_memberships(token_definition_id, community_id)
    where not deleted;