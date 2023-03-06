/* {% require_sudo %} */
alter table galleries add column if not exists name varchar not null default '';
alter table galleries add column if not exists description varchar not null default '';
alter table galleries add column if not exists hidden boolean not null default false;

alter table galleries add column if not exists position varchar;
alter table users add column if not exists featured_gallery varchar;
alter table collections add column if not exists gallery_id varchar references galleries(id);

update galleries set position = 'a0' where position is null;
alter table galleries alter column position set not null;

with ids as (
    select c.id as collection_id, g.id as gallery_id from galleries g, collections c where c.id = any(g.collections) and g.deleted = false
)
update collections set gallery_id = ids.gallery_id from ids where collections.id = ids.collection_id;

update collections set gallery_id = (select id from galleries where collections.owner_user_id = galleries.owner_user_id order by galleries.deleted asc limit 1) where gallery_id is null;
alter table collections alter column gallery_id set not null;

create unique index if not exists position_idx on galleries (owner_user_id, position) where deleted = false;