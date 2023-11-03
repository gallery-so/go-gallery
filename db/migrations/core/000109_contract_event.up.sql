alter table events add column contract_id varchar(255) references contracts(id);
alter table notifications add column contract_id varchar(255) references contracts(id);