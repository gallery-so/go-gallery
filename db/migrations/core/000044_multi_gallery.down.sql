alter table galleries drop column if exists name;
alter table galleries drop column if exists description;
alter table galleries drop column if exists hidden;
alter table galleries drop column if exists position;
alter table users drop column if exists featured_gallery;
alter table collections drop column if exists gallery_id;
drop index if exists position_idx;