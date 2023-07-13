create table if not exists posts (
    id varchar(255) primary key,
    version int not null default 0,
    token_ids varchar(255)[],
    contract_ids varchar(255)[],
    actor_id varchar(255) references users(id) not null,
    caption varchar,
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    deleted boolean not null default false
);

create index if not exists contract_ids_idx on posts using gin (contract_ids);
create index if not exists actor_id_idx on posts (actor_id);

alter table admires add column post_id varchar(255) references posts(id);
alter table comments add column post_id varchar(255) references posts(id);
alter table events add column post_id varchar(255) references posts(id);
alter table notifications add column post_id varchar(255) references posts(id);

alter table admires
add constraint post_feed_event_admire_check check (
    (post_id is not null and feed_event_id is null) or 
    (post_id is null and feed_event_id is not null) or
    (post_id is null and feed_event_id is null)
);

alter table admires 
alter column feed_event_id drop not null;

create index if not exists admire_post_idx on admires (post_id);
create unique index if not exists admire_actor_post_idx on admires (actor_id, post_id) where (deleted = false);
create unique index if not exists admires_created_at_id_post_id_idx on admires (created_at desc, id desc, feed_event_id) where (deleted = false);

alter table comments
add constraint post_feed_event_comment_check check (
    (post_id is not null and feed_event_id is null) or 
    (post_id is null and feed_event_id is not null) or
    (post_id is null and feed_event_id is null)
);

alter table comments 
alter column feed_event_id drop not null;

create index if not exists comment_post_idx on comments (post_id);
create unique index if not exists comments_created_at_id_post_id_idx on comments (created_at desc, id desc, feed_event_id) WHERE (deleted = false);

drop index if exists events_feed_interactions_idx;
create index events_feed_interactions_idx on events (created_at) WHERE (((action)::text = ANY (ARRAY[('CommentedOnFeedEvent'::character varying)::text, ('AdmiredFeedEvent'::character varying)::text, ('CommentedOnPost'::character varying)::text, ('AdmiredPost'::character varying)::text])) and (feed_event_id is not null or post_id is not null));

create view feed_entities as (
SELECT subquery.id, subquery.feed_entity_type, subquery.created_at, subquery.actor_id

    FROM (
        (
            SELECT id, 0 as feed_entity_type, event_time as created_at, owner_id as actor_id
            FROM feed_events
            WHERE deleted = false
        )
        UNION ALL
        (
            SELECT id, 1 as feed_entity_type, created_at, actor_id
            FROM posts
            WHERE deleted = false
        )
    ) subquery
);

