CREATE TABLE IF NOT EXISTS contracts (
    id character varying(255) PRIMARY KEY,
    deleted boolean DEFAULT false NOT NULL,
    version integer,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    last_updated timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    name character varying,
    symbol character varying,
    address character varying(255),
    creator_address character varying(255),
    chain integer,
    latest_block bigint
);

CREATE TABLE IF NOT EXISTS tokens (
    id character varying(255) PRIMARY KEY,
    deleted boolean DEFAULT false NOT NULL,
    version integer,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    last_updated timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    name character varying,
    description character varying,
    contract_address character varying(255),
    media jsonb,
    owner_address character varying(255),
    token_uri character varying,
    token_type character varying,
    token_id character varying,
    quantity character varying,
    ownership_history jsonb[],
    token_metadata jsonb,
    external_url character varying,
    block_number bigint,
    chain integer,
    is_spam boolean
);

CREATE UNIQUE INDEX IF NOT EXISTS address_idx ON contracts USING btree (address);
CREATE INDEX IF NOT EXISTS block_number_idx ON tokens USING btree (block_number);
CREATE INDEX IF NOT EXISTS contract_address_idx ON tokens USING btree (contract_address);
CREATE UNIQUE INDEX IF NOT EXISTS erc1155_idx ON tokens USING btree (token_id, contract_address, owner_address) WHERE ((token_type)::text = 'ERC-1155'::text);
CREATE UNIQUE INDEX IF NOT EXISTS erc721_idx ON tokens USING btree (token_id, contract_address) WHERE ((token_type)::text = 'ERC-721'::text);
CREATE INDEX IF NOT EXISTS owner_address_idx ON tokens USING btree (owner_address);
CREATE INDEX IF NOT EXISTS token_id_contract_address_idx ON tokens USING btree (token_id, contract_address);