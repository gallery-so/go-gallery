/* {% require_sudo %} */
alter role access_rw_pii with login;
grant usage on schema cron to access_rw_pii;