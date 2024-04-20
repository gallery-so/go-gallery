/* {% require_sudo %} */
------------------------------------------------------------------------------------
-- PERMISSIONS MIGRATIONS: sets up standard access roles and login roles
------------------------------------------------------------------------------------

------------------------------------------------------------------------------------
-- Set up access roles and appropriate permissions
------------------------------------------------------------------------------------

-- Create schemas
create schema if not exists ethereum;
create schema if not exists base;
create schema if not exists zora;

-- Limit all access to defined roles
revoke all on schema public from public;
revoke all on schema ethereum from public;
revoke all on schema base from public;
revoke all on schema zora from public;

-- Create access roles
drop role if exists access_ro;
create role access_ro;

drop role if exists access_rw;
create role access_rw;

-- Grant all access roles to postgres so it can function as a superuser, even in
-- Cloud SQL databases (where it's not an actual superuser)
grant access_ro to postgres;
grant access_rw to postgres;

--------------------------------------------------------------------------------------
-- access_ro: has read-only access to all schemas
--------------------------------------------------------------------------------------
-- Grant usage on all schemas
grant usage on schema public to access_ro;

-- Grant read-only privileges for all future objects created by access_rw in the schemas
alter default privileges for role access_rw in schema public grant select on tables to access_ro;
alter default privileges for role access_rw in schema public grant usage on sequences to access_ro;
alter default privileges for role access_rw in schema public grant execute on functions to access_ro;

alter default privileges for role access_rw in schema ethereum grant select on tables to access_ro;
alter default privileges for role access_rw in schema ethereum grant usage on sequences to access_ro;
alter default privileges for role access_rw in schema ethereum grant execute on functions to access_ro;

alter default privileges for role access_rw in schema base grant select on tables to access_ro;
alter default privileges for role access_rw in schema base grant usage on sequences to access_ro;
alter default privileges for role access_rw in schema base grant execute on functions to access_ro;

alter default privileges for role access_rw in schema zora grant select on tables to access_ro;
alter default privileges for role access_rw in schema zora grant usage on sequences to access_ro;
alter default privileges for role access_rw in schema zora grant execute on functions to access_ro;

-- Grant read-only privileges on all current objects in the public schema
grant select on all tables in schema public to access_ro;
grant usage on all sequences in schema public to access_ro;
grant execute on all functions in schema public to access_ro;

--------------------------------------------------------------------------------------
-- access_rw: has read/write access to all current and future objects in all schemas
--------------------------------------------------------------------------------------
-- Grant usage + create privileges on all schemas
grant all on schema public to access_rw;
grant all on schema ethereum to access_rw;
grant all on schema base to access_rw;
grant all on schema zora to access_rw;

-- Make sure access_rw owns all schemas
alter schema public owner to access_rw;
alter schema ethereum owner to access_rw;
alter schema base owner to access_rw;
alter schema zora owner to access_rw;

-- No need to alter default privileges for access_rw in the schemas; it will own newly created objects

-- Grant read/write privileges on all current objects in the schemas
grant all on all tables in schema public to access_rw;
grant all on all sequences in schema public to access_rw;
grant all on all functions in schema public to access_rw;

grant all on all tables in schema ethereum to access_rw;
grant all on all sequences in schema ethereum to access_rw;
grant all on all functions in schema ethereum to access_rw;

grant all on all tables in schema base to access_rw;
grant all on all sequences in schema base to access_rw;
grant all on all functions in schema base to access_rw;

grant all on all tables in schema zora to access_rw;
grant all on all sequences in schema zora to access_rw;
grant all on all functions in schema zora to access_rw;

------------------------------------------------------------------------------------
-- Create standard team login roles (note: before these can be used, a password
-- must be set in the GCP management console)
------------------------------------------------------------------------------------
drop role if exists gallery_team_ro;
create user gallery_team_ro noinherit login;
grant access_ro to gallery_team_ro;
alter role gallery_team_ro set role to access_ro;

drop role if exists gallery_team_rw;
create user gallery_team_rw noinherit login;
grant access_rw to gallery_team_rw;
alter role gallery_team_rw set role to access_rw;

------------------------------------------------------------------------------------
-- Create login roles for services and migrations (note: before these can be used,
-- a password must be set in the GCP management console)
------------------------------------------------------------------------------------
-- gallery_backend is used by our backend services. Defaults to access_rw.
drop role if exists gallery_backend;
create role gallery_backend noinherit login;
grant access_rw to gallery_backend;
alter role gallery_backend set role to access_rw;

-- gallery_migrator is used for migrations. Defaults to access_rw.
drop role if exists gallery_migrator;
create user gallery_migrator noinherit login;
grant access_rw to gallery_migrator;
alter role gallery_migrator set role to access_rw;

