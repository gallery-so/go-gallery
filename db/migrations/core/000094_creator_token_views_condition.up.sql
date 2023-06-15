create or replace view contract_creators as
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
        where c.deleted = false
            and (u.id is not null or coalesce(nullif(c.owner_address, ''), nullif(c.creator_address, '')) is not null);