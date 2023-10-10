alter table contracts add column l1_chain int;
update contracts set l1_chain = 0;
alter table contracts alter column l1_chain set not null;
update contracts set l1_chain = 4 where chain = 4;
create index contracts_l1_chain_idx on contracts (address,chain,l1_chain) where deleted = false;
create unique index contracts_l1_chain_unique_idx on contracts (address,l1_chain) where deleted = false;

drop view if exists contract_creators;

create view contract_creators as
    select c.id as contract_id,
           u.id as creator_user_id,
           c.chain as chain,
           coalesce(nullif(c.owner_address, ''), nullif(c.creator_address, '')) as creator_address
    from contracts c
        left join wallets w on
            w.deleted = false and
            w.l1_chain = c.l1_chain and
            coalesce(nullif(c.owner_address, ''), nullif(c.creator_address, '')) = w.address
        left join users u on
            u.deleted = false and
            (
                (c.override_creator_user_id is not null and c.override_creator_user_id = u.id)
                or
                (c.override_creator_user_id is null and w.address is not null and array[w.id] <@ u.wallets)
            )
        where c.deleted = false;