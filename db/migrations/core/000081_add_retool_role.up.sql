/* {% require_sudo %} */
drop role if exists gallery_tools_retool;
create role gallery_tools_retool noinherit login;
grant access_rw to gallery_tools_retool;
alter role gallery_tools_retool set role to access_rw;
