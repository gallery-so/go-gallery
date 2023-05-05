set role to access_rw_pii;

select cron.schedule('purge-account-creation-info', '@weekly', 'delete from pii.account_creation_info where created_at < now() - interval ''180 days''');

set role to access_rw;