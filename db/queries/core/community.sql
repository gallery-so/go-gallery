-- name: UpsertCommunities :many
insert into communities(id, version, name, description, community_type, key1, key2, key3, key4, profile_image_url, badge_url, contract_id, created_at, last_updated, deleted) (
    select unnest(@ids::varchar[])
         , unnest(@version::int[])
         , unnest(@name::varchar[])
         , unnest(@description::varchar[])
         , unnest(@community_type::int[])
         , unnest(@key1::varchar[])
         , unnest(@key2::varchar[])
         , unnest(@key3::varchar[])
         , unnest(@key4::varchar[])
         , nullif(unnest(@profile_image_url::varchar[]), '')
         , nullif(unnest(@badge_url::varchar[]), '')
         , nullif(unnest(@contract_id::varchar[]), '')
         , now()
         , now()
         , false
)
on conflict (community_type, key1, key2, key3, key4) where not deleted
    do update set version = excluded.version
                , name = coalesce(nullif(excluded.name, ''), nullif(communities.name, ''), '')
                , description = coalesce(nullif(excluded.description, ''), nullif(communities.description, ''), '')
                , profile_image_url = coalesce(nullif(excluded.profile_image_url, ''), nullif(communities.profile_image_url, ''))
                , badge_url = coalesce(nullif(excluded.badge_url, ''), nullif(communities.badge_url, ''))
                , contract_id = coalesce(nullif(excluded.contract_id, ''), nullif(communities.contract_id, ''))
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
         , unnest(@token_definition_id::varchar[]) as token_definition_id
         , unnest(@community_id::varchar[]) as community_id
         , now() as created_at
         , now() as last_updated
         , false as deleted
),
valid_memberships as (
    select memberships.*
    from memberships
    join communities on communities.id = memberships.community_id and not communities.deleted
    join token_definitions on token_definitions.id = memberships.token_definition_id and not token_definitions.deleted
)
insert into token_community_memberships(id, token_definition_id, community_id, created_at, last_updated, deleted) (
    select * from valid_memberships
)
on conflict (community_id, token_definition_id) where not deleted
    do nothing
returning *;

-- name: GetCommunityByID :batchone
select * from communities
    where id = @id
        and not deleted;

-- name: GetCommunityByKey :batchone
select * from communities
    where @type = community_type
        and @key1 = key1
        and @key2 = key2
        and @key3 = key3
        and @key4 = key4
        and not deleted;

-- name: GetCommunitiesByKeys :many
-- dataloader-config: skip=true
-- Get communities by keys
with keys as (
    select unnest (@types::int[]) as type
         , unnest (@key1::varchar[]) as key1
         , unnest (@key2::varchar[]) as key2
         , unnest (@key3::varchar[]) as key3
         , unnest (@key4::varchar[]) as key4
         , generate_subscripts(@types::varchar[], 1) as batch_key_index
)
select k.batch_key_index, sqlc.embed(c) from keys k
    join communities c on
        k.type = c.community_type
        and k.key1 = c.key1
        and k.key2 = c.key2
        and k.key3 = c.key3
        and k.key4 = c.key4
    where not c.deleted;

-- name: GetCommunityContractProviders :many
select * from community_contract_providers
    where contract_id = any(@contract_ids)
    and not deleted;

-- name: UpsertCommunityContractProviders :exec
with entries as (
    select unnest(@ids::varchar[]) as id
         , unnest(@contract_id::varchar[]) as contract_id
         , unnest(@community_type::int[]) as community_type
         , unnest(@is_valid_provider::bool[]) as is_valid_provider
         , now() as created_at
         , now() as last_updated
         , false as deleted
)
insert into community_contract_providers(id, contract_id, community_type, is_valid_provider, created_at, last_updated, deleted) (
    select * from entries
)
on conflict (contract_id, community_type) where not deleted
    do update set is_valid_provider = excluded.is_valid_provider
                , last_updated = now()
returning *;

-- name: PaginatePostsByCommunityID :batchmany
with community_data as (
    select community_type, contract_id
    from communities
    where communities.id = @community_id and not deleted
)

(
select posts.*
    from community_data, posts
    where community_data.community_type = 0
        and community_data.contract_id = any(posts.contract_ids)
        and posts.deleted = false
        and (posts.created_at, posts.id) < (sqlc.arg('cur_before_time'), sqlc.arg('cur_before_id'))
        and (posts.created_at, posts.id) > (sqlc.arg('cur_after_time'), sqlc.arg('cur_after_id'))
    order by
        case when sqlc.arg('paging_forward')::bool then (posts.created_at, posts.id) end asc,
        case when not sqlc.arg('paging_forward')::bool then (posts.created_at, posts.id) end desc
    limit sqlc.arg('limit')
)

union all

(
select posts.*
    from community_data, posts
        join tokens on tokens.id = any(posts.token_ids) and not tokens.deleted
        join token_community_memberships on tokens.token_definition_id = token_community_memberships.token_definition_id
            and token_community_memberships.community_id = @community_id
            and not token_community_memberships.deleted
    where community_data.community_type = 1
      and posts.deleted = false
      and (posts.created_at, posts.id) < (sqlc.arg('cur_before_time'), sqlc.arg('cur_before_id'))
      and (posts.created_at, posts.id) > (sqlc.arg('cur_after_time'), sqlc.arg('cur_after_id'))
    order by
        case when sqlc.arg('paging_forward')::bool then (posts.created_at, posts.id) end asc,
        case when not sqlc.arg('paging_forward')::bool then (posts.created_at, posts.id) end desc
    limit sqlc.arg('limit')
);

-- name: CountPostsByCommunityID :one
with community_data as (
    select community_type, contract_id
    from communities
    where communities.id = @community_id and not deleted
),

community_posts as (
    (
        select posts.*
            from community_data, posts
            where community_data.community_type = 0
                and community_data.contract_id = any(posts.contract_ids)
                and posts.deleted = false
    )

    union all

    (
        select posts.*
            from community_data, posts
                join tokens on tokens.id = any(posts.token_ids) and not tokens.deleted
                join token_community_memberships on tokens.token_definition_id = token_community_memberships.token_definition_id
                    and token_community_memberships.community_id = @community_id
                    and not token_community_memberships.deleted
            where community_data.community_type = 1
              and posts.deleted = false
    )
)

select count(*) from community_posts;

-- name: GetCreatorsByCommunityID :batchmany
select u.id as creator_user_id,
    cc.creator_address as creator_address,
    cc.creator_address_chain as creator_address_chain
    from community_creators cc
        left join wallets w on
            w.deleted = false and
            w.l1_chain = cc.creator_address_l1_chain and
            cc.creator_address = w.address
        left join users u on
            u.deleted = false and
            u.universal = false and
            (
                (cc.creator_user_id is not null and cc.creator_user_id = u.id)
                or
                (cc.creator_user_id is null and w.address is not null and array[w.id] <@ u.wallets)
            )
    where cc.community_id = @community_id
        and cc.deleted = false
    order by (cc.creator_type, cc.creator_user_id, cc.creator_address);

-- name: PaginateHoldersByCommunityID :batchmany
-- Note: sqlc has trouble recognizing that the output of the "select distinct" subquery below will
--       return complete rows from the users table. As a workaround, aliasing the subquery to
--       "users" seems to fix the issue (along with aliasing the users table inside the subquery
--       to "u" to avoid confusion -- otherwise, sqlc creates a custom row type that includes
--       all users.* fields twice).
with community_data as (
    select community_type, contract_id
    from communities
    where communities.id = @community_id and not deleted
),

community_tokens as (
    select tokens.*
    from community_data, tokens
    where community_data.community_type = 0
        and tokens.contract_id = community_data.contract_id
        and not tokens.deleted

    union all

    select tokens.*
    from community_data, tokens
        join token_community_memberships on tokens.token_definition_id = token_community_memberships.token_definition_id
            and token_community_memberships.community_id = @community_id
            and not token_community_memberships.deleted
    where community_data.community_type != 0
        and not tokens.deleted
)

select users.* from (
    select distinct on (u.id) u.* from users u, community_tokens t
        where t.owner_user_id = u.id
        and t.displayable
        and u.universal = false
        and t.deleted = false and u.deleted = false
    ) as users
    where (users.created_at,users.id) < (@cur_before_time::timestamptz, @cur_before_id)
    and (users.created_at,users.id) > (@cur_after_time::timestamptz, @cur_after_id)
    order by case when @paging_forward::bool then (users.created_at,users.id) end asc,
         case when not @paging_forward::bool then (users.created_at,users.id) end desc limit sqlc.narg('limit');


-- name: CountHoldersByCommunityID :one
with community_data as (
    select community_type, contract_id
    from communities
    where communities.id = @community_id and not deleted
),

community_tokens as (
    select tokens.*
    from community_data, tokens
    where community_data.community_type = 0
        and tokens.contract_id = community_data.contract_id
        and not tokens.deleted

    union all

    select tokens.*
    from community_data, tokens
        join token_community_memberships on tokens.token_definition_id = token_community_memberships.token_definition_id
            and token_community_memberships.community_id = @community_id
            and not token_community_memberships.deleted
    where community_data.community_type != 0
        and not tokens.deleted
)

select count(distinct u.id) from users u, community_tokens t
    where t.owner_user_id = u.id
    and t.displayable
    and u.universal = false
    and t.deleted = false and u.deleted = false;

-- name: PaginateTokensByCommunityID :batchmany
with community_data as (
    select community_type, contract_id
    from communities
    where communities.id = @community_id and not deleted
),

community_token_ids as (
    select tokens.id
    from community_data, tokens
    where community_data.community_type = 0
        and tokens.contract_id = community_data.contract_id
        and not tokens.deleted

    union all

    select tokens.id
    from community_data, tokens
        join token_community_memberships on tokens.token_definition_id = token_community_memberships.token_definition_id
            and token_community_memberships.community_id = @community_id
            and not token_community_memberships.deleted
    where community_data.community_type != 0
        and not tokens.deleted
)

select sqlc.embed(t), sqlc.embed(td), sqlc.embed(c) from community_token_ids ct
    join tokens t on t.id = ct.id
    join token_definitions td on t.token_definition_id = td.id
    join users u on u.id = t.owner_user_id
    join contracts c on t.contract_id = c.id
    where t.displayable
    and t.deleted = false
    and c.deleted = false
    and td.deleted = false
    and u.universal = false
    and (t.created_at,t.id) < (@cur_before_time::timestamptz, @cur_before_id)
    and (t.created_at,t.id) > (@cur_after_time::timestamptz, @cur_after_id)
    order by case when @paging_forward::bool then (t.created_at,t.id) end asc,
             case when not @paging_forward::bool then (t.created_at,t.id) end desc
    limit sqlc.arg('limit');

-- name: CountTokensByCommunityID :one
with community_data as (
    select community_type, contract_id
    from communities
    where communities.id = @community_id and not deleted
),

community_tokens as (
    select tokens.*
    from community_data, tokens
    where community_data.community_type = 0
        and tokens.contract_id = community_data.contract_id
        and not tokens.deleted

    union all

    select tokens.*
    from community_data, tokens
        join token_community_memberships on tokens.token_definition_id = token_community_memberships.token_definition_id
            and token_community_memberships.community_id = @community_id
            and not token_community_memberships.deleted
    where community_data.community_type != 0
        and not tokens.deleted
)

select count(t.*) from community_tokens t
    join token_definitions td on t.token_definition_id = td.id
    join users u on u.id = t.owner_user_id
    join contracts c on t.contract_id = c.id
    where t.displayable
    and t.deleted = false
    and c.deleted = false
    and td.deleted = false
    and u.universal = false;

-- name: UpsertCommunityCreators :many
with entries as (
    select unnest(@ids::varchar[]) as id
         , unnest(@community_id::varchar[]) as community_id
         , unnest(@creator_type::int[]) as creator_type
         , nullif(unnest(@creator_user_id::varchar[]), '') as creator_user_id
         , nullif(unnest(@creator_address::varchar[]), '') as creator_address
         , unnest(@creator_address_chain::int[]) as creator_address_chain
         , unnest(@creator_address_l1_chain::int[]) as creator_address_l1_chain
         , now() as created_at
         , now() as last_updated
         , false as deleted
)

insert into community_creators(id, community_id, creator_type, creator_user_id, creator_address, creator_address_chain, creator_address_l1_chain, created_at, last_updated, deleted) (
    select * from entries
)
on conflict do nothing
returning *;

-- name: IsMemberOfCommunity :one
with community_data as (
    select community_type, contract_id
    from communities
    where communities.id = @community_id and not deleted
),

community_token_definitions as (
    select td.*
    from community_data, token_definitions td
    where community_data.community_type = 0
        and td.contract_id = community_data.contract_id
        and not td.deleted

    union all

    select td.*
    from community_data, token_definitions td
        join token_community_memberships on td.id = token_community_memberships.token_definition_id
            and token_community_memberships.community_id = @community_id
            and not token_community_memberships.deleted
    where community_data.community_type != 0
        and not td.deleted
)

select exists(
    select 1
    from tokens, community_token_definitions
    where tokens.owner_user_id = @user_id
      and not tokens.deleted
      and tokens.displayable
      and tokens.token_definition_id = community_token_definitions.id
);