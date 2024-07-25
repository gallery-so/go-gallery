/* {% require_sudo %} */

-- Create schema
create schema if not exists moshicam;

-- Limit all access to defined roles
revoke all on schema moshicam from public;

alter default privileges for role access_rw in schema moshicam grant select on tables to access_ro;
alter default privileges for role access_rw in schema moshicam grant usage on sequences to access_ro;
alter default privileges for role access_rw in schema moshicam grant execute on functions to access_ro;

--------------------------------------------------------------------------------------
-- access_rw: has read/write access to all current and future objects in all schemas
--------------------------------------------------------------------------------------
-- Grant usage + create privileges on all schemas
grant all on schema moshicam to access_rw;

-- Make sure access_rw owns all schemas
alter schema moshicam owner to access_rw;

-- Grant read/write privileges on all current objects in the schemas
grant all on all tables in schema moshicam to access_rw;
grant all on all sequences in schema moshicam to access_rw;
grant all on all functions in schema moshicam to access_rw;

