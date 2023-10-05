alter table nonces add column l1_chain int;
update nonces set l1_chain = 0;
alter table nonces alter column l1_chain set not null;
alter table wallets add column l1_chain int;
update wallets set l1_chain = 0;
alter table wallets alter column l1_chain set not null;
update nonces set l1_chain = 4 where chain = 4;
update wallets set l1_chain = 4 where chain = 4;

create index nonces_l1_chain_idx on nonces (address,chain,l1_chain);
create index wallets_l1_chain_idx on wallets (address,chain,l1_chain);
create unique index wallets_l1_chain_unique_idx on wallets (address,l1_chain) where deleted = false;