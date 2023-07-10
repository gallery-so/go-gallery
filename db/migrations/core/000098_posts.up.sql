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
alter table events add column post_id varchar(255) references posts(id);

alter table admires
add constraint post_feed_event_admire_check check (
    (post_id is not null and feed_event_id is null) or 
    (post_id is null and feed_event_id is not null) or
    (post_id is null and feed_event_id is null)
);

alter table comments
add constraint post_feed_event_comment_check check (
    (post_id is not null and feed_event_id is null) or 
    (post_id is null and feed_event_id is not null) or
    (post_id is null and feed_event_id is null)
);

alter table events
add constraint post_feed_event_check check (
    (post_id is not null and feed_event_id is null) or 
    (post_id is null and feed_event_id is not null)
);