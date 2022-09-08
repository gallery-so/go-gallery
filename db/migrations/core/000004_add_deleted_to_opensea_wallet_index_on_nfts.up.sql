BEGIN;

DROP INDEX IF EXISTS opensea_id_owner_address_inx;
CREATE UNIQUE INDEX opensea_id_owner_address_inx ON nfts (opensea_id, owner_address) WHERE NOT deleted;

COMMIT;