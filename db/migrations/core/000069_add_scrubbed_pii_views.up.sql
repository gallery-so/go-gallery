set role to access_rw_pii;

drop view if exists scrubbed_pii.for_users;
create view scrubbed_pii.for_users as (
    with socials_kvp as (
        -- Redundant jsonb_each because sqlc throws an error if we select "(jsonb_each(pii_socials)).*"
        select user_id, (jsonb_each(pii_socials)).key as key, (jsonb_each(pii_socials)).value as value from pii.for_users
    ),

    socials_scrubbed as (
        select user_id, socials_kvp.key as k,
        case
            when (socials_kvp.value -> 'display')::bool then socials_kvp.value
            else '{"display":false, "metadata":{}}'::jsonb ||
                jsonb_build_object('provider', socials_kvp.value -> 'provider') ||
                jsonb_build_object('id', users.username_idempotent || '-dummy-id')
        end as v
        from socials_kvp join users on socials_kvp.user_id = users.id
    ),

    socials_aggregated as (
        select user_id, jsonb_object_agg(k, v) as socials
            from socials_scrubbed
            group by user_id
    ),

    -- includes social data when display = true, otherwise makes a dummy id and omits metadata
    scrubbed_socials as (
        select for_users.user_id, coalesce(socials_aggregated.socials, '{}'::jsonb) as scrubbed_socials from pii.for_users
            left join socials_aggregated on socials_aggregated.user_id = for_users.user_id
    ),

    -- <username>@dummy-email.gallery.so for users who have email addresses, null otherwise
    scrubbed_email_address as (
        select u.id as user_id,
               case
                   when p.pii_email_address is not null then u.username_idempotent || '@dummy-email.gallery.so'
               end as scrubbed_address from users u, pii.for_users p
        where u.id = p.user_id
    )

    -- Doing this limit 0 union ensures we have appropriate column types for our view
    (select * from pii.for_users limit 0)
    union all
    select p.user_id, e.scrubbed_address, p.deleted, s.scrubbed_socials
        from pii.for_users p
            join scrubbed_email_address e on e.user_id = p.user_id
            join scrubbed_socials s on s.user_id = p.user_id
);