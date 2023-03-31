create table if not exists contract_subgroups (
  id varchar(255) primary key,
  creator_id varchar(255) references users(id),
  parent_id varchar(255) references contracts(id),
  external_id varchar(255),
  name varchar,
  description varchar,
  creator_address varchar(255),
  created_at timestamptz not null default current_timestamp,
  last_updated timestamptz not null default current_timestamp,
  deleted boolean default false not null,
  version int not null default 0,
  unique(creator_id, parent_id, external_id)
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
