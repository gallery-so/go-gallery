create index if not exists community_creators_community_id_idx on community_creators(community_id) where not deleted;
