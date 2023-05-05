/* {% require_sudo %} */
alter table users add column user_experiences jsonb not null default '{}'::jsonb;