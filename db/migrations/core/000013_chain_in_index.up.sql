/* {% require_sudo %} */
DROP INDEX IF EXISTS  token_id_contract_address_owner_user_id_idx;

DROP INDEX IF EXISTS  token_id_contract_address_idx;

CREATE UNIQUE INDEX IF NOT EXISTS token_id_contract_address_chain_owner_user_id_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS, CHAIN, OWNER_USER_ID) WHERE DELETED = false;

CREATE INDEX IF NOT EXISTS token_id_contract_address_chain_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS, CHAIN);
