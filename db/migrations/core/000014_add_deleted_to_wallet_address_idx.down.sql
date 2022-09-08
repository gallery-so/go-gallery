BEGIN;

DROP INDEX IF EXISTS wallet_address_chain_idx;
CREATE UNIQUE INDEX wallet_address_idx ON wallets (address, chain);

COMMIT;