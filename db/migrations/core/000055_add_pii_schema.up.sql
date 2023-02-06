create schema if not exists pii;

alter table pii_for_users set schema pii;
alter table users_with_pii set schema pii;

-- sqlc type will be "PiiForUser"
alter table pii.pii_for_users rename to for_users;

-- sqlc type will be "PiiUserView"
alter table pii.users_with_pii rename to user_view;