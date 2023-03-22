create table if not exists pii.account_creation_info (
    user_id varchar(255) primary key references users(id),
    ip_address text not null,
    created_at timestamptz not null
);

select cron.schedule('purge-account-creation-info', '@weekly', 'delete from pii.account_creation_info where created_at < now() - interval ''180 days''');