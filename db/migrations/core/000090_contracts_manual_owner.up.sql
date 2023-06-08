alter table contracts add column if not exists override_owner_user_id varchar(255);

create view contract_owners as
    select c.id as contract_id, u.id as owner_user_id, c.chain as chain, coalesce(nullif(c.owner_address, ''), nullif(c.creator_address, '')) as owner_address from contracts c
        left join wallets w on
            w.deleted = false and
            w.chain = c.chain and
            coalesce(c.owner_address, c.creator_address) = w.address
        left join users u on
            u.deleted = false and
            (
                c.override_owner_user_id = u.id
                or
                (c.override_owner_user_id is null and w.address is not null and coalesce(nullif(c.owner_address, ''), nullif(c.creator_address, '')) = w.address and array[w.id] <@ u.wallets)
            )
        where c.deleted = false;

create view token_owners as
    select t.id as token_id,
        t.owner_user_id as owner_user_id,
        t.owned_by_wallets && u.wallets as owner_is_holder,
        t.owner_user_id = co.owner_user_id and t.owner_user_id is not null as owner_is_creator
    from tokens t
        inner join users u on t.owner_user_id = u.id and u.deleted = false
        left join contract_owners co on t.contract = co.contract_id
    where t.deleted = false
        and (
            t.owned_by_wallets && u.wallets
            or
            (t.owner_user_id = co.owner_user_id and t.owner_user_id is not null)
        );