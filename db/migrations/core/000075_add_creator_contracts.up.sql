create table if not exists contract_subgroups (
  id varchar(255) primary key,
  creator_address varchar(255) string,
  parent_id varchar(255) references contracts(id),
  external_id varchar(255),
  name varchar,
  description varchar,
  created_at timestamptz not null default current_timestamp,
  last_updated timestamptz not null default current_timestamp,
  deleted boolean default false not null,
  version not null default 0,
  unique(creator_address, parent_id)
);

create table if not exists token_subgroups (
  id varchar(255) primary key,
  token_id varchar(255) references tokens(id),
  subgroup_id varchar(255) references contract_subgroups(id),
  created_at timestamptz not null default current_timestamp,
  last_updated timestamptz not null default current_timestamp,
  deleted boolean default false not null,
  unique(token_id, subgroup_id)
);

create index if not exists contract_subgroup_creator_idx on contract_subgroups(creator_id) where deleted = false;
create index if not exists contract_subgroup_parent_idx on contract_subgroups(parent_id) where deleted = false;
create index if not exists contract_subgroup_tokens_idx on token_subgroups(subgroup_id) where deleted = false;
