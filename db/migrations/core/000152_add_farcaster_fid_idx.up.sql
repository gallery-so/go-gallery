/* {% require_sudo %} */
create index for_users_farcaster_fid_idx on pii.for_users(((pii_socials -> 'Farcaster'::text) ->> 'id'::text) text_ops);
