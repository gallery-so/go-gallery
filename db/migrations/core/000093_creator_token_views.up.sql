alter table contracts add column if not exists override_creator_user_id varchar(255);

create view contract_creators as
    select c.id as contract_id,
           u.id as creator_user_id,
           c.chain as chain,
           coalesce(nullif(c.owner_address, ''), nullif(c.creator_address, '')) as creator_address
    from contracts c
        left join wallets w on
            w.deleted = false and
            w.chain = c.chain and
            coalesce(nullif(c.owner_address, ''), nullif(c.creator_address, '')) = w.address
        left join users u on
            u.deleted = false and
            (
                (c.override_creator_user_id is not null and c.override_creator_user_id = u.id)
                or
                (c.override_creator_user_id is null and w.address is not null and array[w.id] <@ u.wallets)
            )
        where c.deleted = false;

create view token_ownership as
    select t.id as token_id,
        t.owner_user_id as owner_user_id,
        (t.owned_by_wallets && u.wallets)::bool as is_holder,
        (co.creator_user_id is not null and t.owner_user_id = co.creator_user_id)::bool as is_creator
    from tokens t
        inner join users u on t.owner_user_id = u.id and u.deleted = false
        left join contract_creators co on t.contract = co.contract_id
    where t.deleted = false
        and (
            t.owned_by_wallets && u.wallets
            or
            (co.creator_user_id is not null and t.owner_user_id = co.creator_user_id)
        );