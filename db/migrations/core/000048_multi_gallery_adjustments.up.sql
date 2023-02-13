drop index if exists position_idx;

update users set featured_gallery = (select id from galleries where galleries.owner_user_id = users.id and galleries.deleted = false limit 1) where deleted = false;
