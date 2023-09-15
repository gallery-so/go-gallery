-- name: UpsertCommunities :many
insert into communities(id, version, name, description, community_type, community_subtype, community_key, created_at, last_updated, deleted) (
    select unnest(@ids::varchar[])
         , unnest(@version::int[])
         , unnest(@name::varchar[])
         , unnest(@description::varchar[])
         , unnest(@community_type::int[])
         , unnest(@community_subtype::varchar[])
         , unnest(@community_key::varchar[])
         , now()
         , now()
         , false
)
on conflict (community_type, community_subtype, community_key) where not deleted
    do update set version = excluded.version
                , name = coalesce(nullif(excluded.name, ''), nullif(communities.name, ''), '')
                , description = coalesce(nullif(excluded.description, ''), nullif(communities.description, ''), '')
                , last_updated = now()
                , deleted = excluded.deleted
returning *;

-- name: UpsertContractCommunityMemberships :many
with memberships as (
    select unnest(@ids::varchar[]) as id
         , unnest(@contract_id::varchar[]) as contract_id
         , unnest(@community_id::varchar[]) as community_id
         , now() as created_at
         , now() as last_updated
         , false as deleted
),
valid_memberships as (
    select memberships.*
    from memberships
    join communities on communities.id = memberships.community_id and not communities.deleted
    join contracts on contracts.id = memberships.contract_id and not contracts.deleted
)
insert into contract_community_memberships(id, contract_id, community_id, created_at, last_updated, deleted) (
    select * from valid_memberships
)
on conflict (community_id, contract_id) where not deleted
    do nothing
returning *;

-- name: UpsertTokenCommunityMemberships :many
with memberships as (
    select unnest(@ids::varchar[]) as id
         , unnest(@token_id::varchar[]) as token_id
         , unnest(@community_id::varchar[]) as community_id
         , now() as created_at
         , now() as last_updated
         , false as deleted
),
valid_memberships as (
    select memberships.*
    from memberships
    join communities on communities.id = memberships.community_id and not communities.deleted
    join tokens on tokens.id = memberships.token_id and not tokens.deleted
)
insert into token_community_memberships(id, token_id, community_id, created_at, last_updated, deleted) (
    select * from valid_memberships
)
on conflict (community_id, token_id) where not deleted
    do nothing
returning *;

-- name: GetCommunitiesByKeysWithPreservedOrder :many
-- Get communities by keys, and preserve the ordering of the results
with keys as (
    select unnest (@types::int[]) as type
         , unnest (@subtypes::varchar[]) as subtype
         , unnest (@keys::varchar[]) as key
         , generate_subscripts(@types::varchar[], 1) as idx
)
select c.* from keys k
    join communities c on
        k.type = c.community_type
        and k.subtype = c.community_subtype
        and k.key = c.community_key
    where not c.deleted
    order by k.idx;

