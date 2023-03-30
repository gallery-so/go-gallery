create table if not exists omnibus_contracts (
  id varchar(255) primary key,
  creator_id  varchar(255) references users(id),
  parent_id varchar(255) references contracts(id),
  external_id varchar(255),
  created_at timestamptz not null default current_timestamp,
  last_updated timestamptz not null default current_timestamp,
  deleted boolean default false not null,
  unique(creator_id, parent_id, external_id, deleted)
);

create table if not exists omnibus_tokens (
  id varchar(255) primary key,
  token_id references tokens(id),
  omnibus_id references omnibus_contracts(id),
  created_at timestamptz not null default current_timestamp,
  last_updated timestamptz not null default current_timestamp,
  deleted boolean default false not null,
  unique(token_id, omnibus_id, deleted)
);

create index if not exists omnibus_contracts_creator_idx on omnibus_contracts(creator_id) where deleted = false;
create index if not exists omnibus_contracts_parent_idx on omnibus_contracts(parent_id) where deleted = false;
create index if not exists omnibus_tokens_omnibus_idx on omnibus_tokens(omnibus_id) where deleted = false;
