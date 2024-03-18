create table if not exists highlight_mint_claims (
  id varchar(255) primary key,
  user_id varchar(255) references users(id),
  chain int,
  contract_address varchar,
  token_mint_id numeric,
  token_metadata jsonb,
  recipient_wallet_id varchar not null references wallets(id),
  highlight_collection_id varchar not null,
  token_id varchar(255) references tokens(id),
  claim_id varchar not null unique,
  status varchar not null,
  error_message varchar,
  created_at timestamptz not null default current_timestamp,
  last_updated timestamptz not null default current_timestamp,
  deleted boolean not null default false
);
create unique index highlight_mint_claims_chain_contract_token_unique on highlight_mint_claims (chain, contract_address, token_mint_id) where token_mint_id is not null;
create index highlight_mint_claims_user_id_collection_id on highlight_mint_claims(user_id, highlight_collection_id);
