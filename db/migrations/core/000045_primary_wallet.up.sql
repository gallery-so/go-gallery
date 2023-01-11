alter table users add column primary_wallet_id varchar(255) references wallets(id);

-- every user has at least 1 wallet right?
update users set primary_wallet_id = wallets[0];

alter table users alter column primary_wallet_id set not null;