/* {% require_sudo %} */
drop role if exists gallery_retool;
create role gallery_retool noinherit login;
grant access_rw to gallery_retool;
alter role gallery_retool set role to access_rw;
