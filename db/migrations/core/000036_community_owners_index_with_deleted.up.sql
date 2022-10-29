drop index if exists tokens_contract_owner_user_id_idx;
create index tokens_contract_owner_user_id_idx on tokens (contract, owner_user_id) where deleted = false;