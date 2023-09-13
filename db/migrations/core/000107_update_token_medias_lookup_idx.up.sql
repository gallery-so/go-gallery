create index if not exists token_medias_token_identifiers_idx on token_medias(chain, contract_id, token_id) where not deleted;
drop index if exists token_media_contract_token_id_chain_idx;

