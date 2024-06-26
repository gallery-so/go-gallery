create index if not exists ethereum_contracts_address on ethereum.contracts (address);
create index if not exists base_contracts_address on base.contracts (address);
create index if not exists zora_contracts_address on zora.contracts (address);
create index if not exists base_sepolia_contracts_address on base_sepolia.contracts (address);

create index if not exists ethereum_contracts_owned_by on ethereum.contracts (owned_by);
create index if not exists base_contracts_owned_by on base.contracts (owned_by);
create index if not exists zora_contracts_owned_by on zora.contracts (owned_by);
create index if not exists base_sepolia_contracts_owned_by on base_sepolia.contracts (owned_by);