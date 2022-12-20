alter table galleries add column if not exists name varchar not null default '';
alter table galleries add column if not exists description varchar not null default '';
alter table galleries add column if not exists hidden boolean not null default false;
alter table galleries add column if not exists position varchar not null;
alter table users add column if not exists featured_gallery varchar;
alter table collections add column if not exists gallery_id varchar not null references galleries(id);

create unique index if not exists position_idx on galleries (owner_user_id, position) where deleted = false;