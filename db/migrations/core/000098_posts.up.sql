create table if not exists posts (
    id varchar(255) primary key,
    version int not null default 0,
    token_ids varchar(255)[],
    actor_id varchar(255) references users(id) not null,
    caption varchar,
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp,
    deleted boolean not null default false
);

alter table admires add column post_id varchar(255) references posts(id);
alter table comments add column post_id varchar(255) references posts(id);


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