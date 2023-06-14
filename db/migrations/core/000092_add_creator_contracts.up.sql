alter table contracts add column if not exists parent_id varchar(255) references contracts(id);
create index if not exists contracts_parent_idx on contracts(parent_id) where deleted = false;
create unique index if not exists contracts_chain_address_idx on contracts(chain, address) where parent_id is null;
create unique index if not exists contracts_chain_parent_address_idx on contracts(chain, parent_id, address) where parent_id is not null;
drop index if exists contract_address_chain_idx;
