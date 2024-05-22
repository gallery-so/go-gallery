/* {% require_sudo %} */

-- Create schema
create schema if not exists base_sepolia;

-- Limit all access to defined roles
revoke all on schema base_sepolia from public;

alter default privileges for role access_rw in schema base_sepolia grant select on tables to access_ro;
alter default privileges for role access_rw in schema base_sepolia grant usage on sequences to access_ro;
alter default privileges for role access_rw in schema base_sepolia grant execute on functions to access_ro;

--------------------------------------------------------------------------------------
-- access_rw: has read/write access to all current and future objects in all schemas
--------------------------------------------------------------------------------------
-- Grant usage + create privileges on all schemas
grant all on schema base_sepolia to access_rw;

-- Make sure access_rw owns all schemas
alter schema base_sepolia owner to access_rw;

-- Grant read/write privileges on all current objects in the schemas
grant all on all tables in schema base_sepolia to access_rw;
grant all on all sequences in schema base_sepolia to access_rw;
grant all on all functions in schema base_sepolia to access_rw;

