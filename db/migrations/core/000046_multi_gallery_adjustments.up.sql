drop index if exists position_idx;
create index if not exists position_idx on galleries (owner_user_id, position) where deleted = false;
alter table galleries add constraint position_cst unique (owner_user_id, position) deferrable;

update users set featured_gallery = (select id from galleries where galleries.owner_user_id = users.id and galleries.deleted = false limit 1) where deleted = false;
