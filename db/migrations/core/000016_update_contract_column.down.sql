ALTER TABLE tokens DROP COLUMN IF EXISTS contract;
ALTER TABLE tokens ADD COLUMN contract_address varchar(255);
DROP INDEX IF EXISTS token_id_contract_chain_owner_user_id_idx;
DROP INDEX IF EXISTS token_id_contract_chain_idx;
CREATE UNIQUE INDEX IF NOT EXISTS token_id_contract_address_chain_owner_user_id_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS, CHAIN, OWNER_USER_ID) WHERE DELETED = false;
CREATE INDEX IF NOT EXISTS token_id_contract_address_chain_idx ON tokens (TOKEN_ID, CONTRACT_ADDRESS, CHAIN);

