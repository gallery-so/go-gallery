/* {% require_sudo %} */
create index users_wallets_idx on users using gin(wallets) where deleted = false;