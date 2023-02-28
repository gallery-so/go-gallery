------------------------------------------------------------------------------------
-- NOTE: Should be applied after the pg_cron extension is created. If the extension
--       hasn't been made, run the below statement with the superuser role:
--           `create extension if not exists pg_cron;
--            alter schema cron owner to access_rw;
--            alter role access_rw with login;`
------------------------------------------------------------------------------------
select cron.schedule('daily', 'refresh materialized view concurrently top_recommended_users with data');
