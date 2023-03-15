set role to access_rw_pii;

create view scrubbed_pii.for_users as (
    -- includes social data when display = true, otherwise makes a dummy id and omits metadata
    with scrubbed_socials as (
        with socials as (
            select user_id, (jsonb_each(pii_socials)).* from pii.for_users
        ),

        scrubbed as (
            select user_id, key as k,
            case
                when (socials.value -> 'display')::bool then socials.value
                else '{"display":false, "metadata":{}}'::jsonb ||
                    jsonb_build_object('provider', socials.value -> 'provider') ||
                    jsonb_build_object('id', users.username_idempotent || '-dummy-id')
            end as v
            from socials join users on socials.user_id = users.id
        ),

        aggregated as (
            select user_id, jsonb_object_agg(k, v) as socials
                from scrubbed
                group by user_id
        )

        select for_users.user_id, coalesce(aggregated.socials, '{}'::jsonb) as scrubbed_socials from pii.for_users
            left join aggregated on aggregated.user_id = for_users.user_id
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