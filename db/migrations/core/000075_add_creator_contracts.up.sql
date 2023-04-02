alter table contracts add column if not exists parent_id varchar(255) references contracts(id);
create index if not exists contracts_parent_idx on contracts(id) where deleted = false;
alter table tokens add column if not exists child_contract_id varchar(255) references contracts(id);
create index if not exists tokens_child_contract_idx on tokens(child_contract_id) where deleted = false;
