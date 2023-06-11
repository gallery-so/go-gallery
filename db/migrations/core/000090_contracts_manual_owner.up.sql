alter table contracts add column if not exists override_owner_user_id varchar(255);

create view contract_owners as
    -- This view is essentially making its own nullable types because sqlc doesn't generate appropriate
    -- nullable types here on its own. For example, if we just return u.id as owner_user_id, sqlc generates
    -- a persist.DBID, even though the column could actually be null.
    select c.id as contract_id,
           coalesce(u.id, '') as owner_user_id,
           (u.id is not null)::bool as owner_user_id_valid,
           c.chain as chain,
           coalesce(nullif(c.owner_address, ''), nullif(c.creator_address, ''), '') as owner_address,
           (coalesce(nullif(c.owner_address, ''), nullif(c.creator_address, '')) is not null)::bool as owner_address_valid
    from contracts c
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
        (t.owned_by_wallets && u.wallets)::bool as owner_is_holder,
        (co.has_owner_user_id and t.owner_user_id = co.owner_user_id)::bool as owner_is_creator
    from tokens t
        inner join users u on t.owner_user_id = u.id and u.deleted = false
        left join contract_owners co on t.contract = co.contract_id
    where t.deleted = false
        and (
            t.owned_by_wallets && u.wallets
            or
            (co.owner_user_id is not null and t.owner_user_id = co.owner_user_id)
        );