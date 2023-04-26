create table if not exists alchemy_spam_contracts (
  id varchar(255) primary key,
  chain integer not null,
  address varchar(255) not null,
  created_at timestamptz not null,
  is_spam bool not null default false,
  unique(chain, address)
);

create index if not exists alchemy_spam_contracts_chain_address_idx on alchemy_spam_contracts(chain, address);
