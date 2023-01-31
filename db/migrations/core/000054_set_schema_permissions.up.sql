------------------------------------------------------------------------------------
-- PERMISSIONS MIGRATIONS: sets up standard access roles and login roles
------------------------------------------------------------------------------------
-- NOTE: This migration must be run as the default `postgres' superuser, but all
--       migrations after this one should be run as `gallery_migrator`
------------------------------------------------------------------------------------

------------------------------------------------------------------------------------
-- Set up access roles and appropriate permissions
------------------------------------------------------------------------------------

-- Create pii schema
create schema if not exists public;
create schema if not exists pii;

-- Limit all access to defined roles
revoke all on schema public from public;
revoke all on schema pii from public;

-- Create access roles
drop role if exists access_ro;
create role access_ro;

drop role if exists access_ro_pii;
create role access_ro_pii;

drop role if exists access_rw;
create role access_rw;

drop role if exists access_rw_pii;
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

-- Grant read-only privileges on all current objects in the public schema
grant select on all tables in schema public to access_ro;
grant usage on all sequences in schema public to access_ro;
grant execute on all functions in schema public to access_ro;

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

-- Grant read-only privileges for all current objects in the public schema
grant select on all tables in schema public to access_ro_pii;
grant usage on all sequences in schema public to access_ro_pii;
grant execute on all functions in schema public to access_ro_pii;

-- Grant read-only privileges for all current objects in the pii schema
grant select on all tables in schema pii to access_ro_pii;
grant usage on all sequences in schema pii to access_ro_pii;
grant execute on all functions in schema pii to access_ro_pii;

--------------------------------------------------------------------------------------
-- access_rw: has read/write access to all current and future objects in public schema
--------------------------------------------------------------------------------------
-- Grant usage + create privileges on the public schema
grant all on schema public to access_rw;

-- Make sure access_rw owns the public schema
alter schema public owner to access_rw;

-- No need to alter default privileges for access_rw in the public schema; it will own newly created objects

-- Grant read/write privileges on all current objects in the public schema
grant all on all tables in schema public to access_rw;
grant all on all sequences in schema public to access_rw;
grant all on all functions in schema public to access_rw;

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

-- Grant read/write privileges on all current objects in the public schema
grant all on all tables in schema public to access_rw_pii;
grant all on all sequences in schema public to access_rw_pii;
grant all on all functions in schema public to access_rw_pii;

-- Grant read/write privileges on all current objects in the pii schema
grant all on all tables in schema pii to access_rw_pii;
grant all on all sequences in schema pii to access_rw_pii;
grant all on all functions in schema pii to access_rw_pii;

------------------------------------------------------------------------------------
-- Create standard team login roles (note: before these can be used, a password
-- must be set in the GCP management console)
------------------------------------------------------------------------------------
drop role if exists gallery_team_ro;
create user gallery_team_ro noinherit login;
grant access_ro to gallery_team_ro;
alter role gallery_team_ro set role to access_ro;

drop role if exists gallery_team_ro_pii;
create user gallery_team_ro_pii noinherit login;
grant access_ro to gallery_team_ro_pii;
grant access_ro_pii to gallery_team_ro_pii;
-- pii roles log in without pii access, but can use "set role" to gain access when needed
alter role gallery_team_ro_pii set role to access_ro;

drop role if exists gallery_team_rw;
create user gallery_team_rw noinherit login;
grant access_rw to gallery_team_rw;
alter role gallery_team_rw set role to access_rw;

drop role if exists gallery_team_rw_pii;
create user gallery_team_rw_pii noinherit login;
grant access_rw to gallery_team_rw_pii;
grant access_rw_pii to gallery_team_rw_pii;
-- pii roles log in without pii access, but can use "set role" to gain access when needed
alter role gallery_team_rw_pii set role to access_rw;

------------------------------------------------------------------------------------
-- Create login roles for services and migrations (note: before these can be used,
-- a password must be set in the GCP management console)
------------------------------------------------------------------------------------
-- gallery_backend is used by our backend services. Defaults to access_rw_pii.
drop role if exists gallery_backend;
create role gallery_backend noinherit login;
grant access_rw to gallery_backend;
grant access_rw_pii to gallery_backend;
alter role gallery_backend set role to access_rw_pii;

-- gallery_migrator is used for migrations. Defaults to access_rw, but can assume
-- access_rw_pii to create or migrate pii tables.
drop role if exists gallery_migrator;
create user gallery_migrator noinherit login;
grant access_rw to gallery_migrator;
grant access_rw_pii to gallery_migrator;
alter role gallery_migrator set role to access_rw;

------------------------------------------------------------------------------------
------------------------------------------------------------------------------------
------------------------------------------------------------------------------------
-- Anything above this section is generic and applicable to any gallery database.
-- The remainder of the migration is specific to the existing tables in our main
-- database.
------------------------------------------------------------------------------------
------------------------------------------------------------------------------------
------------------------------------------------------------------------------------

------------------------------------------------------------------------------------
-- Transfer ownership of existing public tables to access_rw
------------------------------------------------------------------------------------
alter table access owner to access_rw;
alter table admires owner to access_rw;
alter table collection_events owner to access_rw;
alter table collections owner to access_rw;
alter table comments owner to access_rw;
alter table contracts owner to access_rw;
alter table dev_metadata_users owner to access_rw;
alter table early_access owner to access_rw;
alter table events owner to access_rw;
alter table features owner to access_rw;
alter table feed_blocklist owner to access_rw;
alter table feed_events owner to access_rw;
alter table follows owner to access_rw;
alter table galleries owner to access_rw;
alter table legacy_views owner to access_rw;
alter table login_attempts owner to access_rw;
alter table membership owner to access_rw;
alter table merch owner to access_rw;
alter table nft_events owner to access_rw;
alter table nonces owner to access_rw;
alter table notifications owner to access_rw;
alter table schema_migrations owner to access_rw;
alter table tokens owner to access_rw;
alter table user_events owner to access_rw;
alter table user_roles owner to access_rw;
alter table users owner to access_rw;
alter table wallets owner to access_rw;

------------------------------------------------------------------------------------
-- Set permissions for existing pii tables
------------------------------------------------------------------------------------
-- Revoke existing pii table privileges from access_ro and access_rw
revoke all privileges on pii.for_users from access_ro;
revoke all privileges on pii.for_users from access_rw;
revoke all privileges on pii.user_view from access_ro;
revoke all privileges on pii.user_view from access_rw;

-- Grant read-only privileges to access_ro_pii
grant select on pii.for_users to access_ro_pii;
grant select on pii.user_view to access_ro_pii;

-- Grant read/write privileges to access_rw_pii
grant all on pii.for_users to access_rw_pii;
grant all on pii.user_view to access_rw_pii;

-- Transfer ownership of existing pii tables to access_rw_pii
alter table pii.for_users owner to access_rw_pii;
alter table pii.user_view owner to access_rw_pii;

