alter table token_medias add column chain int;
alter table token_medias add column contract_address varchar;
alter table token_medias add column token_id numeric;
create index token_medias_chain_contract_token_idx on token_medias(chain, contract_address, token_id) where not deleted;
