alter table nonces add column l1_chain int not null default 0;
alter table wallets add column l1_chain int not null default 0;

update nonces set l1_chain = 4 where chain = 4;
update wallets set l1_chain = 4 where chain = 4;