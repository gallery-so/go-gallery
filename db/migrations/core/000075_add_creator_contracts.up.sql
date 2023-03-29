create table if not exists creator_contracts (
  id varchar(255) primary key,
  creator_id  varchar(255) references users(id),
  contract_id varchar(255) references contracts(id),
  created_at timestamptz not null default current_timestamp,
  last_updated timestamptz not null default current_timestamp,
  deleted boolean default false not null,
  unique(creator_id, contract_id, deleted)
);

create index if not exists creator_contracts_creator_idx on creator_contracts(creator_id) where deleted = false;
create index if not exists creator_contracts_contract_idx on creator_contracts(contract_id) where deleted = false;
