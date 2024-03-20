create table if not exists highlight_mint_claims (
  id varchar(255) primary key,
  recipient_user_id varchar(255) not null references users(id),
  recipient_l1_chain integer not null,
  recipient_address varchar not null,
  recipient_wallet_id varchar not null references wallets(id),	
  internal_token_id varchar(255) references tokens(id), -- populated when the mint is fully processed
  highlight_collection_id varchar not null,
  highlight_claim_id varchar not null unique,
  collection_address varchar not null,
  collection_chain int not null,
  minted_token_id varchar, -- populated when the tx succeeds
  minted_token_metadata jsonb, -- populated when the tx succeeds
  status varchar not null,
  error_message varchar,
  created_at timestamptz not null default current_timestamp,
  last_updated timestamptz not null default current_timestamp,
  deleted boolean not null default false
);
create unique index highlight_mint_claims_chain_contract_token on highlight_mint_claims (collection_chain, collection_address, minted_token_id) where minted_token_id is not null and not deleted;
create unique index highlight_mint_claims_user_collection on highlight_mint_claims(recipient_l1_chain, recipient_user_id, highlight_collection_id) where not deleted;
create unique index highlight_mint_claims_recipient_wallet_collection on highlight_mint_claims(recipient_l1_chain, recipient_address, highlight_collection_id) where not deleted;
