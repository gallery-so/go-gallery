alter table galleries drop column name;
alter table galleries drop column description;
alter table galleries drop column hidden;
alter table galleries drop column position;
alter table users drop column featured_gallery;
drop index if exists position_idx;