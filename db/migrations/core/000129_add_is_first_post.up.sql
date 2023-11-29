begin;
alter table posts add column if not exists is_first_post boolean not null default false;
create unique index if not exists posts_actor_id_is_first_post on posts(actor_id, is_first_post) where is_first_post;
-- populate is_first_post
with first_posts as (select id from posts p where not exists(select 1 from posts where created_at < p.created_at and p.actor_id = actor_id limit 1))
update posts set is_first_post = true from first_posts where first_posts.id = posts.id;
end;
