------------------------------------------------------------------------------------
-- NOTE: Migrations that schedule jobs should be applied after the pg_cron
--       extension is created. If the extension hasn't beeen made, run the below
--       statement with the superuser role:
--           `create extension if not exists pg_cron;
--            alter schema cron owner to access_rw;`
------------------------------------------------------------------------------------
select cron.schedule('daily', 'refresh materialized view concurrently top_recommended_users with data');
