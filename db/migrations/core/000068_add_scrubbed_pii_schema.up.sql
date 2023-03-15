/* {% require_sudo %} */

create schema if not exists scrubbed_pii;
alter schema scrubbed_pii owner to access_rw_pii;

-- All access levels get read access to scrubbed_pii views, but only access_rw_pii can create
-- or modify views. In practical terms, access_rw_pii can set up scrubbed views that query pii
-- tables but don't return any actual pii, and other access levels can query (but not modify)
-- those views.
grant usage on schema scrubbed_pii to access_ro;
alter default privileges for role access_rw_pii in schema scrubbed_pii grant select on tables to access_ro;
alter default privileges for role access_rw_pii in schema scrubbed_pii grant usage on sequences to access_ro;
alter default privileges for role access_rw_pii in schema scrubbed_pii grant execute on functions to access_ro;

grant usage on schema scrubbed_pii to access_rw;
alter default privileges for role access_rw_pii in schema scrubbed_pii grant select on tables to access_rw;
alter default privileges for role access_rw_pii in schema scrubbed_pii grant usage on sequences to access_rw;
alter default privileges for role access_rw_pii in schema scrubbed_pii grant execute on functions to access_rw;

grant usage on schema scrubbed_pii to access_ro_pii;
alter default privileges for role access_rw_pii in schema scrubbed_pii grant select on tables to access_ro_pii;
alter default privileges for role access_rw_pii in schema scrubbed_pii grant usage on sequences to access_ro_pii;
alter default privileges for role access_rw_pii in schema scrubbed_pii grant execute on functions to access_ro_pii;