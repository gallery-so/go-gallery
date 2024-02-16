alter table token_definitions add column if not exists provider varchar;
alter table token_definitions add column if not exists provider_priority varchar not null default 'a0' collate "C";
