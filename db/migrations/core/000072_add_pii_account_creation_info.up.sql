set role to access_rw_pii;

create table if not exists pii.account_creation_info (
    user_id varchar(255) primary key references users(id),
    ip_address text not null,
    created_at timestamptz not null
);

-- Create a scrubbed_pii view of the table
drop view if exists scrubbed_pii.account_creation_info;
create view scrubbed_pii.account_creation_info as (
    -- Doing this limit 0 union ensures we have appropriate column types for our view
    (select * from pii.account_creation_info limit 0)
    union
    select user_id, 'scrubbed', created_at from pii.account_creation_info
);

set role to access_rw;