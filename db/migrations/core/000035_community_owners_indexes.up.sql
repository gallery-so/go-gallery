/* {% require_sudo %} */
create index tokens_contract_owner_user_id_idx on tokens (contract, owner_user_id);