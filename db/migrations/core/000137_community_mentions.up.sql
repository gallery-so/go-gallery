alter table mentions add column community_id varchar(255) references communities(id);
alter table events add column community_id varchar(255) references communities(id);
alter table notifications add column community_id varchar(255) references communities(id);

-- Migrate mentions.contract_id to mentions.community_id
with remapped as (
    select contracts.id as contract_id,
           communities.id as community_id
        from contracts,
             communities
        where contracts.id = communities.contract_id
          and communities.community_type = 0
          and communities.deleted = false
          and contracts.deleted = false
)
update mentions
    set community_id = remapped.community_id
    from remapped
    where mentions.contract_id = remapped.contract_id;

-- Migrate events.contract_id to events.community_id
with remapped as (
    select contracts.id as contract_id,
           communities.id as community_id
        from contracts,
             communities
        where contracts.id = communities.contract_id
          and communities.community_type = 0
          and communities.deleted = false
          and contracts.deleted = false
)
update events
    set community_id = remapped.community_id
    from remapped
    where events.contract_id = remapped.contract_id;

-- Migrate notifications.contract_id to notifications.community_id
with remapped as (
    select contracts.id as contract_id,
           communities.id as community_id
        from contracts,
             communities
        where contracts.id = communities.contract_id
          and communities.community_type = 0
          and communities.deleted = false
          and contracts.deleted = false
)
update notifications
    set community_id = remapped.community_id
    from remapped
    where notifications.contract_id = remapped.contract_id;


-- Drop the old contract_id columns now that the community_id columns are populated
alter table mentions drop column contract_id;
alter table events drop column contract_id;
alter table notifications drop column contract_id;
