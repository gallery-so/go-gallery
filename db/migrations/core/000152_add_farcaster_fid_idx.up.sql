/* {% require_sudo %} */
create index for_users_farcaster_fid_idx on pii.for_users(((pii_socials -> 'farcaster'::text) ->> 'id'::text) text_ops);
