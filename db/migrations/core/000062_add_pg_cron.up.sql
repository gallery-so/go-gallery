/* {% require_sudo %} */
create extension if not exists pg_cron;
alter schema cron owner to access_rw;
alter role access_rw with login;
