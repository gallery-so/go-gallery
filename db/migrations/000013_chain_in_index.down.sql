CREATE UNIQUE INDEX IF NOT EXISTS token_id_contract_address_owner_user_id_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS, OWNER_USER_ID);

CREATE INDEX IF NOT EXISTS token_id_contract_address_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS);

DROP INDEX IF EXISTS token_id_contract_address_chain_owner_user_id_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS, CHAIN,OWNER_USER_ID);

DROP INDEX IF EXISTS token_id_contract_address_chain_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS, CHAIN);
