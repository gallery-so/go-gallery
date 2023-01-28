------------------------------------------------------------------------------------
-- PERMISSIONS MIGRATIONS: sets up standard access roles and login roles
------------------------------------------------------------------------------------
-- NOTE: This migration must be run as the default `postgres' superuser, but all
--       migrations after this one should be run as `gallery_migrator`
------------------------------------------------------------------------------------

------------------------------------------------------------------------------------
-- To set up access roles and appropriate permissions
------------------------------------------------------------------------------------

-- Create pii schema
create schema if not exists pii;

-- Limit all access to defined roles
revoke all on schema public from public;
revoke all on schema pii from public;

-- Create access roles
create role access_ro;
create role access_ro_pii;

create role access_rw;
create role access_rw_pii;

-- Grant all access roles to postgres so it can function as a superuser, even in
-- Cloud SQL databases (where it's not an actual superuser)
grant access_ro to postgres;
grant access_rw to postgres;
grant access_ro_pii to postgres;
grant access_rw_pii to postgres;

--------------------------------------------------------------------------------------
-- access_ro: has read-only access to the public schema
--------------------------------------------------------------------------------------
-- Grant usage on the public schema
grant usage on schema public to access_ro;

-- Grant read-only privileges for all future objects created by access_rw in the public schema
alter default privileges for role access_rw in schema public grant select on tables to access_ro;
alter default privileges for role access_rw in schema public grant usage on sequences to access_ro;
alter default privileges for role access_rw in schema public grant execute on functions to access_ro;

--------------------------------------------------------------------------------------
-- access_ro_pii: has read-only access to objects in both the public and pii schemas
--------------------------------------------------------------------------------------
-- Grant usage on the public schema
grant usage on schema public to access_ro_pii;

-- Grant read-only privileges for all future objects created by access_rw in the public schema
alter default privileges for role access_rw in schema public grant select on tables to access_ro_pii;
alter default privileges for role access_rw in schema public grant usage on sequences to access_ro_pii;
alter default privileges for role access_rw in schema public grant execute on functions to access_ro_pii;

-- Grant usage on the pii schema
grant usage on schema pii to access_ro_pii;

-- Grant read-only privileges for all future objects created by access_rw_pii in the pii schema
alter default privileges for role access_rw_pii in schema pii grant select on tables to access_ro_pii;
alter default privileges for role access_rw_pii in schema pii grant usage on sequences to access_ro_pii;
alter default privileges for role access_rw_pii in schema pii grant execute on functions to access_ro_pii;

--------------------------------------------------------------------------------------
-- access_rw: has read/write access to all current and future objects in public schema
--------------------------------------------------------------------------------------
-- Grant usage + create privileges on the public schema
grant all on schema public to access_rw;

-- Make sure access_rw owns the public schema
alter schema public owner to access_rw;

-- No need to alter default privileges for access_rw in the public schema; it will own newly created objects

--------------------------------------------------------------------------------------
-- access_rw_pii: has read/write access to all current and future objects in both the
--                public schema and pii schema
--------------------------------------------------------------------------------------

-- Grant usage privileges to access_rw_pii for the public schema, but don't give
-- it create permission (because access_rw should own all objects in the public
-- schema, which means it should be the only role creating objects there)
grant usage on schema public to access_rw_pii;

-- Grant read/write privileges for all future objects created by access_rw in the public schema
alter default privileges for role access_rw in schema public grant all on tables to access_rw_pii;
alter default privileges for role access_rw in schema public grant all on sequences to access_rw_pii;
alter default privileges for role access_rw in schema public grant all on functions to access_rw_pii;

-- Grant usage + create privileges on the pii schema
grant all on schema pii to access_rw_pii;

-- Make sure access_rw_pii owns the pii schema
alter schema pii owner to access_rw_pii;

-- No need to alter default privileges for access_rw_pii in pii schema; it will own newly created objects

------------------------------------------------------------------------------------
-- To create standard team login roles (note: before these can be used, a
-- password must be set in the GCP management console)
------------------------------------------------------------------------------------
create user gallery_team_ro noinherit login;
grant access_ro to gallery_team_ro;
alter role gallery_team_ro set role to access_ro;

create user gallery_team_ro_pii noinherit login;
grant access_ro_pii to gallery_team_ro_pii;
-- pii roles log in without pii access, but can use "set role" to gain access when needed
alter role gallery_team_ro_pii set role to access_ro;

create user gallery_team_rw noinherit login;
grant access_rw to gallery_team_rw;
alter role gallery_team_rw set role to access_rw;

create user gallery_team_rw_pii noinherit login;
grant access_rw_pii to gallery_team_rw_pii;
-- pii roles log in without pii access, but can use "set role" to gain access when needed
alter role gallery_team_rw_pii set role to access_rw;

------------------------------------------------------------------------------------
-- To create login roles for services and migrations (note: before these can be used,
-- a password must be set in the GCP management console)
------------------------------------------------------------------------------------
-- gallery_backend is used by our backend services. Defaults to access_rw_pii.
create role gallery_backend noinherit login;
grant access_rw_pii to gallery_backend;
alter role gallery_backend set role to access_rw_pii;

-- gallery_migrator is used for migrations. Defaults to access_rw, but can assume
-- access_rw_pii to create or migrate pii tables.
create user gallery_migrator noinherit login;
grant access_rw_pii to gallery_migrator;
alter role gallery_migrator set role to access_rw;

