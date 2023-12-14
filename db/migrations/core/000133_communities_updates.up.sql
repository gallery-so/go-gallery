drop index if exists community_creators_community_id_type_user_id_address_chain_idx;

create unique index if not exists community_creators_community_id_creator_address_idx
    on community_creators (community_id, creator_type, creator_address, creator_address_l1_chain)
    where not deleted and creator_user_id is null;

create unique index if not exists community_creators_community_id_creator_user_id_idx
    on community_creators (community_id, creator_type, creator_user_id)
    where not deleted and creator_user_id is not null;