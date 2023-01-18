drop index if exists position_idx;
alter table galleries add constraint position_cst unique (owner_user_id, position) deferrable;

update users set featured_gallery = (select id from galleries where galleries.owner_user_id = users.id and galleries.deleted = false limit 1) where deleted = false;
