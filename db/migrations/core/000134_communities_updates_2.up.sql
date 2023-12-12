set role to access_rw;

alter table communities
    add column website_url text,
    add column override_website_url text;