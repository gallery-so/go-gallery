alter table events add column group_id varchar(255);
alter table events add index group_id_idx (group_id);