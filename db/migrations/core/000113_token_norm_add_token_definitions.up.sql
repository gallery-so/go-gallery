-- Ensures that changes to a contract are mirrored in the token_definitions table. A unique constraint on (chain, address) already exists to guarantee uniqueness within the table itself.
create unique index contracts_id_chain_address_idx on contracts(id, chain, address);

create table if not exists token_definitions (
  id character varying(255) primary key,
  created_at timestamp with time zone not null default current_timestamp,
  last_updated timestamp with time zone not null default current_timestamp,
  deleted boolean not null default false,
  name character varying,
  description character varying,
  token_type character varying,
  token_id character varying,
  external_url character varying,
  chain integer,
  metadata jsonb,
  fallback_media jsonb,
  contract_address character varying(255) not null,
  contract_id character varying(255) references contracts(id) not null,
  token_media_id character varying(255) references token_medias(id),
  foreign key(contract_id, chain, contract_address) references contracts(id, chain, address) on update cascade
);
create unique index if not exists token_definitions_chain_contract_id_token_idx on token_definitions(chain, contract_id, token_id) where not deleted;
create unique index if not exists token_definitions_chain_contract_address_token_idx on token_definitions(chain, contract_address, token_id) where not deleted;
create index token_definitions_contract_id_idx on token_definitions(contract_id) where not deleted;
