drop index if exists token_community_memberships_community_id_token_def_id_idx;
create index token_community_memberships_community_definition_token_id
    on token_community_memberships(community_id, token_definition_id, token_id)
    where not deleted;