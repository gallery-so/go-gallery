create index if not exists ethereum_owners_contract_token_quantity on ethereum.owners (contract_address, token_id, quantity desc, first_acquired_date);
create index if not exists base_owners_contract_token_quantity on base.owners (contract_address, token_id, quantity desc, first_acquired_date);
create index if not exists zora_owners_contract_token_quantity on zora.owners (contract_address, token_id, quantity desc, first_acquired_date);
create index if not exists base_sepolia_owners_contract_token_quantity on base_sepolia.owners (contract_address, token_id, quantity desc, first_acquired_date);