-- Dummy email addresses
with dummy_pii as (
    select u.id as id, u.username_idempotent || '@dummy-email.gallery.so' as email
        from users u join dev_metadata_users d on u.id = d.user_id and u.deleted = false and d.deleted = false
)
insert into pii.for_users (select dummy_pii.id, dummy_pii.email, false from dummy_pii);