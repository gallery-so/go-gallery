alter table galleries add column name varchar;
alter table galleries add column description varchar;
alter table galleries add column hidden boolean NOT NULL default false;
alter table galleries add column position varchar NOT NULL default 'm';
alter table users add column featured_gallery varchar;

create unique index if not exists position_idx on galleries (position, owner_user_id) where deleted = false;