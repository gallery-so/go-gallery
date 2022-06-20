BEGIN;

DROP INDEX IF EXISTS wallet_address_idx;
CREATE UNIQUE INDEX wallet_address_chain_idx ON wallets (address, chain) WHERE NOT deleted;

COMMIT;