alter table push_notification_tickets add status text;
update push_notification_tickets set status = 'unknown' where status is null;
alter table push_notification_tickets alter column status set not null;
