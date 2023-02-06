alter table events drop column if exists group_id;
drop index if exists group_id_idx; 