/* {% require_sudo %} */
alter table users add column primary_wallet_id varchar(255) references wallets(id);

-- Delete users that are already marked as deleted that would otherwise
-- violate the NOT NULL constraint
-- N.B. This is a one-way operation. Applying a down migration won't bring back these users.
delete from users where id in (
	-- deleted users with no wallets
	select id from users where cardinality(users.wallets) = 0 and deleted = true
	union
	-- deleted users that have a non-existent wallet
	select users.id from users, unnest(users.wallets) as w left join wallets on w = wallets.id WHERE wallets.id is null and users.deleted = true
);

-- every user has at least 1 wallet right?
update users set primary_wallet_id = wallets[1];

alter table users alter column primary_wallet_id set not null;
