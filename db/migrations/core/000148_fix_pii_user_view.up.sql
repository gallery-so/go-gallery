set role to access_rw_pii;

drop view if exists pii.user_view;
create or replace view pii.user_view as
    select users.id,
           users.deleted,
           users.version,
           users.last_updated,
           users.created_at,
           users.username,
           users.username_idempotent,
           users.wallets,
           users.bio,
           users.traits,
           users.universal,
           users.notification_settings,
           users.email_unsubscriptions,
           users.featured_gallery,
           users.primary_wallet_id,
           users.user_experiences,
           users.profile_image_id,
           for_users.pii_unverified_email_address,
           for_users.pii_verified_email_address,
           for_users.pii_socials
    from users
        left join pii.for_users
            on users.id = for_users.user_id
                and for_users.deleted = false;

set role to access_rw;
