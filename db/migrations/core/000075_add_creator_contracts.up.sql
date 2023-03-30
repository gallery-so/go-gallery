create table if not exists contract_hierarchies (
  id varchar(255) primary key,
  creator_id  varchar(255) references users(id),
  parent_id varchar(255) references contracts(id),
  external_id varchar(255),
  created_at timestamptz not null default current_timestamp,
  last_updated timestamptz not null default current_timestamp,
  deleted boolean default false not null,
  unique(id, parent_id, deleted),
  unique(id, creator_id, deleted)
);

create table if not exists subcontract_tokens (
  id varchar(255) primary key,
  token_id varchar(255) references tokens(id),
  hierarchy_id varchar(255) references contract_hierarchies(id),
  created_at timestamptz not null default current_timestamp,
  last_updated timestamptz not null default current_timestamp,
  deleted boolean default false not null,
  unique(token_id, hierarchy_id, deleted)
);

create index if not exists contract_hierachies_creator_idx on contract_hierarchies(creator_id) where deleted = false;
create index if not exists contract_hierachies_parent_idx on contract_hierarchies(parent_id) where deleted = false;
create index if not exists subcontract_tokens_idx on subcontract_tokens(hierarchy_id) where deleted = false;
