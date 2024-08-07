-- name: GetUserById :one
SELECT * FROM users WHERE id = $1 AND deleted = false;

-- name: GetUserWithPIIByID :one
select * from pii.user_view where id = @user_id and deleted = false;

-- name: GetUserByIdBatch :batchone
SELECT * FROM users WHERE id = $1 AND deleted = false;

-- name: GetUsersByIDs :many
SELECT * FROM users WHERE id = ANY(@user_ids::dbid[]) AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id::dbid)
    AND (created_at, id) > (@cur_after_time, @cur_after_id::dbid)
    ORDER BY CASE WHEN @paging_forward::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT @paging_forward::bool THEN (created_at, id) END DESC
    LIMIT $1;

-- name: GetUserByUsername :one
select * from users where username_idempotent = lower(sqlc.arg('username')) and deleted = false and universal = false;

-- name: GetUserByUsernameBatch :batchone
select * from users where username_idempotent = lower($1) and deleted = false and universal = false;

-- name: GetUserByVerifiedEmailAddress :one
select u.* from users u join pii.for_users p on u.id = p.user_id
where p.pii_verified_email_address = lower($1)
  and p.deleted = false
  and u.deleted = false;

-- name: GetUserByAddressAndL1 :one
select users.*
from users, wallets
where wallets.address = sqlc.arg('address')
	and wallets.l1_chain = sqlc.arg('l1_chain')
	and array[wallets.id] <@ users.wallets
	and wallets.deleted = false
	and users.deleted = false;

-- name: GetUserByAddressAndL1Batch :batchone
select users.*
from users, wallets
where wallets.address = sqlc.arg('address')
	and wallets.l1_chain = sqlc.arg('l1_chain')
	and array[wallets.id] <@ users.wallets
	and wallets.deleted = false
	and users.deleted = false;

-- name: GetUsersWithTrait :many
SELECT * FROM users WHERE (traits->$1::text) IS NOT NULL AND deleted = false;

-- name: GetUsersWithTraitBatch :batchmany
SELECT * FROM users WHERE (traits->$1::text) IS NOT NULL AND deleted = false;

-- name: GetGalleryById :one
SELECT * FROM galleries WHERE id = $1 AND deleted = false;

-- name: GetGalleryByIdBatch :batchone
SELECT * FROM galleries WHERE id = $1 AND deleted = false;

-- name: GetGalleryByCollectionId :one
SELECT g.* FROM galleries g, collections c WHERE c.id = $1 AND c.deleted = false AND $1 = ANY(g.collections) AND g.deleted = false;

-- name: GetGalleryByCollectionIdBatch :batchone
SELECT g.* FROM galleries g, collections c WHERE c.id = $1 AND c.deleted = false AND $1 = ANY(g.collections) AND g.deleted = false;

-- name: GetGalleriesByUserId :many
SELECT * FROM galleries WHERE owner_user_id = $1 AND deleted = false order by position;

-- name: GetGalleriesByUserIdBatch :batchmany
SELECT * FROM galleries WHERE owner_user_id = $1 AND deleted = false order by position;

-- name: GetCollectionById :one
SELECT * FROM collections WHERE id = $1 AND deleted = false;

-- name: GetCollectionByIdBatch :batchone
SELECT * FROM collections WHERE id = $1 AND deleted = false;

-- name: GetCollectionsByGalleryId :many
SELECT c.* FROM galleries g, unnest(g.collections)
    WITH ORDINALITY AS x(coll_id, coll_ord)
    INNER JOIN collections c ON c.id = x.coll_id
    WHERE g.id = $1 AND g.deleted = false AND c.deleted = false ORDER BY x.coll_ord;

-- name: GetCollectionsByGalleryIdBatch :batchmany
SELECT c.* FROM galleries g, unnest(g.collections)
    WITH ORDINALITY AS x(coll_id, coll_ord)
    INNER JOIN collections c ON c.id = x.coll_id
    WHERE g.id = $1 AND g.deleted = false AND c.deleted = false ORDER BY x.coll_ord;

-- name: GetTokenById :one
select sqlc.embed(t), sqlc.embed(td)
from tokens t
join token_definitions td on t.token_definition_id = td.id
where t.id = $1 and t.displayable and t.deleted = false and td.deleted = false;

-- name: GetTokenDefinitionById :one
select * from token_definitions where id = $1 and not deleted;

-- name: GetTokenDefinitionByIdBatch :batchone
select * from token_definitions where id = $1 and not deleted;

-- name: GetTokenDefinitionByTokenDbid :one
select token_definitions.*
from token_definitions, tokens
where token_definitions.id = tokens.token_definition_id
    and tokens.id = $1
    and not tokens.deleted
    and not token_definitions.deleted;

-- name: GetTokenDefinitionByTokenDbidBatch :batchone
select token_definitions.*
from token_definitions, tokens
where token_definitions.id = tokens.token_definition_id
    and tokens.id = $1
    and not tokens.deleted
    and not token_definitions.deleted;

-- name: GetTokenDefinitionByTokenIdentifiers :one
select *
from token_definitions
where (chain, contract_address, token_id) = (@chain, @contract_address::address, @token_id::hextokenid) and not deleted;

-- name: GetTokenFullDetailsByUserTokenIdentifiers :one
select sqlc.embed(tokens), sqlc.embed(token_definitions), sqlc.embed(contracts)
from tokens
join token_definitions on tokens.token_definition_id = token_definitions.id
join contracts on token_definitions.contract_id = contracts.id
where tokens.owner_user_id = $1 
    and (token_definitions.chain, token_definitions.contract_address, token_definitions.token_id) = (@chain, @contract_address::address, @token_id::hextokenid)
    and tokens.displayable
    and not tokens.deleted
    and not token_definitions.deleted
    and not contracts.deleted
order by tokens.block_number desc;

-- name: UpdateTokenCollectorsNoteByTokenDbidUserId :exec
update tokens set collectors_note = $1, last_updated = now() where id = $2 and owner_user_id = $3;

-- name: UpdateTokensAsUserMarkedSpam :exec
update tokens set is_user_marked_spam = $1, last_updated = now() where owner_user_id = $2 and id = any(@token_ids::dbid[]) and deleted = false;

-- name: CheckUserOwnsAllTokenDbids :one
with user_tokens as (select count(*) total from tokens where id = any(@token_ids::dbid[]) and owner_user_id = $1 and not tokens.deleted), total_tokens as (select cardinality(@token_ids) total)
select (select total from total_tokens) = (select total from user_tokens) owns_all;

-- name: GetTokenByIdBatch :batchone
select sqlc.embed(t), sqlc.embed(td)
from tokens t
join token_definitions td on t.token_definition_id = td.id
where t.id = $1 and t.displayable and t.deleted = false and td.deleted = false;

-- name: GetTokenByIdIgnoreDisplayableBatch :batchone
select sqlc.embed(t), sqlc.embed(td)
from tokens t
join token_definitions td on t.token_definition_id = td.id
where t.id = $1 and t.deleted = false and td.deleted = false;

-- name: GetTokenByUserTokenIdentifiersBatch :batchone
-- Fetch the definition and contract to cache since downstream queries will likely use them
select sqlc.embed(t), sqlc.embed(td), sqlc.embed(c)
from tokens t, token_definitions td, contracts c
where t.token_definition_id = td.id
    and td.contract_id = c.id
    and t.owner_user_id = @owner_id
    and td.token_id = @token_id
    and td.chain = @chain
    and td.contract_address = @contract_address
    and t.displayable
    and not t.deleted
    and not td.deleted
    and not c.deleted;

-- name: GetTokenByUserTokenIdentifiersIgnoreDisplayableBatch :batchone
select sqlc.embed(t), sqlc.embed(td), sqlc.embed(c), sqlc.embed(tm)
from tokens t, token_definitions td, contracts c, token_medias tm
where t.token_definition_id = td.id
    and td.contract_id = c.id
    and t.owner_user_id = @owner_id
    and td.token_id = @token_id
    and td.chain = @chain
    and td.contract_address = @contract_address
    and td.token_media_id = tm.id
    and not t.deleted
    and not td.deleted
    and not c.deleted
    and not tm.deleted;

-- name: GetTokenByUserTokenIdentifiers :one
select sqlc.embed(t), sqlc.embed(td), sqlc.embed(c)
from tokens t, token_definitions td, contracts c
where t.token_definition_id = td.id
    and td.contract_id = c.id
    and t.owner_user_id = @owner_id
    and td.token_id = @token_id
    and td.chain = @chain
    and td.contract_address = @contract_address
    and t.displayable
    and not t.deleted
    and not td.deleted
    and not c.deleted;

-- name: GetTokensByCollectionIdBatch :batchmany
select t.* from collections c,
    unnest(c.nfts) with ordinality as u(nft_id, nft_ord)
    join tokens t on t.id = u.nft_id
    where c.id = sqlc.arg('collection_id')
      and c.owner_user_id = t.owner_user_id
      and t.displayable
      and c.deleted = false
      and t.deleted = false
    order by u.nft_ord
    limit sqlc.narg('limit');

-- name: PaginateTokensAdmiredByUserIDBatch :batchmany
select sqlc.embed(tokens), sqlc.embed(admires)
from admires
join tokens on admires.token_id = tokens.id
where actor_id = @user_id and not admires.deleted and not tokens.deleted
and (admires.created_at, admires.id) > (@cur_before_time, @cur_before_id::dbid)
and (admires.created_at, admires.id) < (@cur_after_time, @cur_after_id::dbid)
order by case when sqlc.arg('paging_forward')::bool then (admires.created_at, admires.id) end desc,
    case when not sqlc.arg('paging_forward')::bool then (admires.created_at, admires.id) end asc
limit sqlc.arg('limit');

-- name: CountTokensAdmiredByUserID :one
select count(*) from admires, tokens where actor_id = $1 and admires.token_id = tokens.id and not admires.deleted and not tokens.deleted;

-- name: GetContractCreatorsByIds :many
with keys as (
    select unnest (@contract_ids::text[]) as id
         , generate_subscripts(@contract_ids::text[], 1) as batch_key_index
)
select k.batch_key_index, sqlc.embed(c) from keys k
    join contract_creators c on c.contract_id = k.id;

-- name: GetNewTokensByFeedEventIdBatch :batchmany
with new_tokens as (
    select added.id, row_number() over () added_order
    from (select jsonb_array_elements_text(data -> 'collection_new_token_ids') id from feed_events f where f.id = $1 and f.deleted = false) added
)
select t.* from new_tokens a join tokens t on a.id = t.id and t.displayable and t.deleted = false order by a.added_order;

-- name: GetMembershipByMembershipId :one
SELECT * FROM membership WHERE id = $1 AND deleted = false;

-- name: GetMembershipByMembershipIdBatch :batchone
SELECT * FROM membership WHERE id = $1 AND deleted = false;

-- name: GetWalletByID :one
SELECT * FROM wallets WHERE id = $1 AND deleted = false;

-- name: GetWalletByIDBatch :batchone
SELECT * FROM wallets WHERE id = $1 AND deleted = false;

-- name: GetWalletByAddressAndL1Chain :one
SELECT wallets.* FROM wallets WHERE address = $1 AND l1_chain = $2 AND deleted = false;

-- name: GetWalletsByUserID :many
SELECT w.* FROM users u, unnest(u.wallets) WITH ORDINALITY AS a(wallet_id, wallet_ord)INNER JOIN wallets w on w.id = a.wallet_id WHERE u.id = $1 AND u.deleted = false AND w.deleted = false ORDER BY a.wallet_ord;

-- name: GetWalletsByUserIDBatch :batchmany
SELECT w.* FROM users u, unnest(u.wallets) WITH ORDINALITY AS a(wallet_id, wallet_ord)INNER JOIN wallets w on w.id = a.wallet_id WHERE u.id = $1 AND u.deleted = false AND w.deleted = false ORDER BY a.wallet_ord;

-- name: GetEthereumWalletsForEnsProfileImagesByUserID :many
select w.*
from wallets w
join users u on w.id = any(u.wallets)
where u.id = $1 and w.chain = 0 and not w.deleted
order by u.primary_wallet_id = w.id desc, w.id desc;

-- name: GetContractByID :one
select * FROM contracts WHERE id = $1 AND deleted = false;

-- name: GetContractsByIDs :many
with keys as (
    select unnest (@contract_ids::varchar[]) as id
         , generate_subscripts(@contract_ids::varchar[], 1) as batch_key_index
)
select k.batch_key_index, sqlc.embed(c) from keys k
    join contracts c on c.id = k.id
    where not c.deleted;

-- name: GetContractsByTokenIDs :many
select contracts.* from contracts join tokens on contracts.id = tokens.contract_id where tokens.id = any(@token_ids::dbid[]) and contracts.deleted = false;

-- name: GetContractByChainAddress :one
select * FROM contracts WHERE address = $1 AND chain = $2 AND deleted = false;

-- name: GetContractByChainAddressBatch :batchone
select * FROM contracts WHERE address = $1 AND chain = $2 AND deleted = false;

-- name: GetContractsDisplayedByUserIDBatch :batchmany
with last_refreshed as (
  select last_updated from owned_contracts limit 1
),
displayed as (
  select contract_id
  from owned_contracts
  where owned_contracts.user_id = $1 and displayed = true
  union
  select contracts.id
  from last_refreshed, galleries, contracts, tokens
  join collections on tokens.id = any(collections.nfts) and collections.deleted = false
  where tokens.owner_user_id = $1
    and tokens.contract_id = contracts.id
    and collections.owner_user_id = tokens.owner_user_id
    and galleries.owner_user_id = tokens.owner_user_id
    and tokens.displayable
    and tokens.deleted = false
    and galleries.deleted = false
    and contracts.deleted = false
    and galleries.last_updated > last_refreshed.last_updated
    and collections.last_updated > last_refreshed.last_updated
)
select contracts.* from contracts, displayed
where contracts.id = displayed.contract_id and contracts.deleted = false;

-- name: GetFollowersByUserIdBatch :batchmany
SELECT u.* FROM follows f
    INNER JOIN users u ON f.follower = u.id
    WHERE f.followee = $1 AND f.deleted = false
    ORDER BY f.last_updated DESC;

-- name: GetFollowingByUserIdBatch :batchmany
SELECT u.* FROM follows f
    INNER JOIN users u ON f.followee = u.id
    WHERE f.follower = $1 AND f.deleted = false
    ORDER BY f.last_updated DESC;

-- name: GetTokensByWalletIdsBatch :batchmany
select sqlc.embed(t), sqlc.embed(td)
from tokens t
join token_definitions td on t.token_definition_id = td.id
where t.owned_by_wallets && $1 and t.displayable and t.deleted = false and td.deleted = false
order by t.created_at desc, td.name desc, t.id desc;

-- name: GetTokensByContractAddressUserId :many
select sqlc.embed(t), sqlc.embed(td), sqlc.embed(c)
from tokens t
join token_definitions td on t.token_definition_id = td.id
join contracts c on td.contract_id = c.id
where t.owner_user_id = $1 
    and td.contract_address = $2
    and td.chain = $3
    and @wallet_id::varchar = any(t.owned_by_wallets)
    and not t.deleted
    and not td.deleted;

-- name: GetTokensByContractIdPaginate :many
select sqlc.embed(t), sqlc.embed(td), sqlc.embed(c), sqlc.embed(u) from tokens t
    join token_definitions td on t.token_definition_id = td.id
    join users u on u.id = t.owner_user_id
    join contracts c on t.contract_id = c.id
    where (c.id = $1 or c.parent_id = $1)
    and t.displayable
    and t.deleted = false
    and c.deleted = false
    and td.deleted = false
    and (not @gallery_users_only::bool or u.universal = false)
    and (u.universal,t.created_at,t.id) < (@cur_before_universal, @cur_before_time::timestamptz, @cur_before_id::dbid)
    and (u.universal,t.created_at,t.id) > (@cur_after_universal, @cur_after_time::timestamptz, @cur_after_id::dbid)
    order by case when @paging_forward::bool then (u.universal,t.created_at,t.id) end asc,
             case when not @paging_forward::bool then (u.universal,t.created_at,t.id) end desc
    limit $2;

-- name: CountTokensByContractId :one
select count(*)
from tokens
join users on users.id = tokens.owner_user_id
join contracts on tokens.contract_id = contracts.id
where (contracts.id = @id or contracts.parent_id = @id)
  and (not @gallery_users_only::bool or users.universal = false) and tokens.deleted = false and contracts.deleted = false
  and tokens.displayable;

-- name: GetOwnersByContractIdBatchPaginate :batchmany
-- Note: sqlc has trouble recognizing that the output of the "select distinct" subquery below will
--       return complete rows from the users table. As a workaround, aliasing the subquery to
--       "users" seems to fix the issue (along with aliasing the users table inside the subquery
--       to "u" to avoid confusion -- otherwise, sqlc creates a custom row type that includes
--       all users.* fields twice).
select users.* from (
    select distinct on (u.id) u.* from users u, tokens t, contracts c
        where t.owner_user_id = u.id
        and t.displayable
        and t.contract_id = c.id and (c.id = @id or c.parent_id = @id)
        and (not @gallery_users_only::bool or u.universal = false)
        and t.deleted = false and u.deleted = false and c.deleted = false
    ) as users
    where (users.universal,users.created_at,users.id) < (@cur_before_universal, @cur_before_time::timestamptz, @cur_before_id::dbid)
    and (users.universal,users.created_at,users.id) > (@cur_after_universal, @cur_after_time::timestamptz, @cur_after_id::dbid)
    order by case when @paging_forward::bool then (users.universal,users.created_at,users.id) end asc,
         case when not @paging_forward::bool then (users.universal,users.created_at,users.id) end desc limit sqlc.narg('limit');


-- name: CountOwnersByContractId :one
select count(distinct users.id) from users, tokens, contracts
    where (contracts.id = @id or contracts.parent_id = @id)
    and tokens.contract_id = contracts.id
    and tokens.owner_user_id = users.id
    and tokens.displayable
    and (not @gallery_users_only::bool or users.universal = false)
    and tokens.deleted = false and users.deleted = false and contracts.deleted = false;

-- name: GetPreviewURLsByContractIdAndUserId :many
select coalesce(nullif(tm.media->>'thumbnail_url', ''), nullif(tm.media->>'media_url', ''))::varchar as thumbnail_url
from tokens t
join token_definitions td on t.token_definition_id = td.id
join token_medias tm on td.token_media_id = tm.id
where t.owner_user_id = @owner_id
	and t.contract_id = @contract_id
	and t.displayable
	and (tm.media ->> 'thumbnail_url' != '' or tm.media->>'media_type' = 'image')
	and not t.deleted
	and not td.deleted
	and not tm.deleted
	and tm.active
order by t.id limit 3;

-- name: GetTokensByUserIdBatch :batchmany
select sqlc.embed(t), sqlc.embed(td), sqlc.embed(c)
from tokens t
join token_definitions td on t.token_definition_id = td.id
join contracts c on c.id = td.contract_id
where t.owner_user_id = @owner_user_id
    and t.deleted = false
    and t.displayable
    and ((@include_holder::bool and t.is_holder_token) or (@include_creator::bool and t.is_creator_token))
    and td.deleted = false
    and c.deleted = false
order by t.created_at desc, td.name desc, t.id desc;

-- name: CreateUserEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, user_id, subject_id, post_id, comment_id, feed_event_id, mention_id, data, group_id, caption) VALUES ($1, $2, $3, $4, $5, $5, sqlc.narg('post')::text, sqlc.narg('comment')::text, sqlc.narg('feed_event')::text, sqlc.narg('mention')::text, $6, $7, $8) RETURNING *;

-- name: CreateTokenEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, token_id, subject_id, data, group_id, caption, gallery_id, collection_id) VALUES ($1, $2, $3, $4, $5, $5, $6, $7, $8, sqlc.narg('gallery')::text, sqlc.narg('collection')::text) RETURNING *;

-- name: CreateCollectionEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, collection_id, subject_id, data, caption, group_id, gallery_id) VALUES ($1, $2, $3, $4, $5, $5, $6, $7, $8, $9) RETURNING *;

-- name: CreateGalleryEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, gallery_id, subject_id, data, external_id, group_id, caption) VALUES ($1, $2, $3, $4, $5, $5, $6, $7, $8, $9) RETURNING *;

-- name: CreateCommunityEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, community_id, subject_id, post_id, comment_id, feed_event_id, mention_id, data, group_id, caption) VALUES ($1, $2, $3, $4, $5, $5, sqlc.narg('post')::text, sqlc.narg('comment')::text, sqlc.narg('feed_event')::text, sqlc.narg('mention')::text, $6, $7, $8) RETURNING *;

-- name: CreatePostEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, user_id, subject_id, post_id) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: CreateDataOnlyEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, data, group_id, caption, subject_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: CreateAdmireEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, admire_id, feed_event_id, post_id, token_id, comment_id, subject_id, data, group_id, caption) VALUES ($1, $2, $3, $4, $5, sqlc.narg('feed_event')::text, sqlc.narg('post')::text, sqlc.narg('token')::text, sqlc.narg('comment')::text, $6, $7, $8, $9) RETURNING *;

-- name: CreateCommentEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, comment_id, feed_event_id, post_id, mention_id, subject_id, data, group_id, caption) VALUES ($1, $2, $3, $4, $5, sqlc.narg('feed_event')::text, sqlc.narg('post')::text, sqlc.narg('mention')::text, $6, $7, $8, $9) RETURNING *;

-- name: GetEvent :one
SELECT * FROM events WHERE id = $1 AND deleted = false;

-- name: GetEventsInWindow :many
with recursive activity as (
    select * from events where events.id = $1 and deleted = false
    union
    select e.* from events e, activity a
    where e.actor_id = a.actor_id
        and e.action = any(@actions::action[])
        and e.created_at < a.created_at
        and e.created_at >= a.created_at - make_interval(secs => $2)
        and e.deleted = false
        and e.caption is null
        and (not @include_subject::bool or e.subject_id = a.subject_id)
)
select * from events where id = any(select id from activity) order by (created_at, id) asc;

-- name: GetGalleryEventsInWindow :many
with recursive activity as (
    select * from events where events.id = $1 and deleted = false
    union
    select e.* from events e, activity a
    where e.actor_id = a.actor_id
        and e.action = any(@actions::action[])
        and e.gallery_id = @gallery_id
        and e.created_at < a.created_at
        and e.created_at >= a.created_at - make_interval(secs => $2)
        and e.deleted = false
        and e.caption is null
        and (not @include_subject::bool or e.subject_id = a.subject_id)
)
select * from events where id = any(select id from activity) order by (created_at, id) asc;

-- name: GetEventsInGroup :many
select * from events where group_id = @group_id and deleted = false order by(created_at, id) asc;

-- name: GetActorForGroup :one
select actor_id from events where group_id = @group_id and deleted = false order by(created_at, id) asc limit 1;

-- name: HasLaterGroupedEvent :one
select exists(
  select 1 from events where deleted = false
  and group_id = @group_id
  and id > @event_id
);

-- name: IsActorActionActive :one
select exists(
  select 1 from events where deleted = false
  and actor_id = $1
  and action = any(@actions::action[])
  and created_at > @window_start and created_at <= @window_end
);

-- name: IsActorSubjectActive :one
select exists(
  select 1 from events where deleted = false
  and actor_id = $1
  and subject_id = $2
  and created_at > @window_start and created_at <= @window_end
);

-- name: IsActorGalleryActive :one
select exists(
  select 1 from events where deleted = false
  and actor_id = $1
  and gallery_id = $2
  and created_at > @window_start and created_at <= @window_end
);


-- name: IsActorSubjectActionActive :one
select exists(
  select 1 from events where deleted = false
  and actor_id = $1
  and subject_id = $2
  and action = any(@actions::action[])
  and created_at > @window_start and created_at <= @window_end
);

-- name: PaginateGlobalFeed :many
select fe.*
from feed_entities fe
left join feed_blocklist fb on fe.actor_id = fb.user_id and not fb.deleted and fb.active
where (fe.created_at, fe.id) < (@cur_before_time, @cur_before_id::dbid)
        and (fe.created_at, fe.id) > (@cur_after_time, @cur_after_id::dbid)
        and (fb.user_id is null or @viewer_id = fb.user_id)
order by
    case when sqlc.arg('paging_forward')::bool then (fe.created_at, fe.id) end asc,
    case when not sqlc.arg('paging_forward')::bool then (fe.created_at, fe.id) end desc
limit sqlc.arg('limit');

-- name: PaginatePersonalFeedByUserID :many
select fe.* from feed_entities fe, follows fl
    where fl.deleted = false
      and fe.actor_id = fl.followee
      and fl.follower = sqlc.arg('follower')
      and (fe.created_at, fe.id) < (@cur_before_time, @cur_before_id::dbid)
      and (fe.created_at, fe.id) > (@cur_after_time, @cur_after_id::dbid)
order by
    case when sqlc.arg('paging_forward')::bool then (fe.created_at, fe.id) end asc,
    case when not sqlc.arg('paging_forward')::bool then (fe.created_at, fe.id) end desc
limit sqlc.arg('limit');

-- name: PaginatePostsByUserID :many
select *
from posts
where actor_id = sqlc.arg('user_id')
        and (created_at, id) < (@cur_before_time, @cur_before_id::dbid)
        and (created_at, id) > (@cur_after_time, @cur_after_id::dbid)
        and not posts.deleted
order by
    case when sqlc.arg('paging_forward')::bool then (created_at, id) end asc,
    case when not sqlc.arg('paging_forward')::bool then (created_at, id) end desc
limit sqlc.arg('limit');

-- name: CountPostsByUserID :one
select count(*) from posts where actor_id = $1 and not posts.deleted;

-- name: PaginatePostsByContractID :batchmany
SELECT posts.*
FROM posts
WHERE @contract_id::dbid = ANY(posts.contract_ids)
AND posts.deleted = false
AND (posts.created_at, posts.id) < (@cur_before_time, @cur_before_id::dbid)
AND (posts.created_at, posts.id) > (@cur_after_time, @cur_after_id::dbid)
ORDER BY
    CASE WHEN sqlc.arg('paging_forward')::bool THEN (posts.created_at, posts.id) END ASC,
    CASE WHEN NOT sqlc.arg('paging_forward')::bool THEN (posts.created_at, posts.id) END DESC
LIMIT sqlc.arg('limit');

-- name: PaginatePostsByContractIDAndProjectID :many
with valid_post_ids as (
    SELECT distinct on (posts.id) posts.id
    FROM posts
        JOIN tokens on tokens.id = ANY(posts.token_ids)
            and tokens.displayable
            and tokens.deleted = false
        JOIN token_definitions on token_definitions.id = tokens.token_definition_id
            and token_definitions.contract_id = sqlc.arg('contract_id')
            and ('x' || lpad(substring(token_definitions.token_id, 1, 16), 16, '0'))::bit(64)::bigint / 1000000 = sqlc.arg('project_id_int')::int
            and token_definitions.deleted = false
    WHERE sqlc.arg('contract_id') = ANY(posts.contract_ids)
      AND posts.deleted = false
)
SELECT posts.* from posts
    join valid_post_ids on posts.id = valid_post_ids.id
WHERE (posts.created_at, posts.id) < (@cur_before_time, @cur_before_id::dbid)
  AND (posts.created_at, posts.id) > (@cur_after_time, @cur_after_id::dbid)
ORDER BY
    CASE WHEN sqlc.arg('paging_forward')::bool THEN (posts.created_at, posts.id) END ASC,
    CASE WHEN NOT sqlc.arg('paging_forward')::bool THEN (posts.created_at, posts.id) END DESC
LIMIT sqlc.arg('limit');

-- name: CountPostsByContractID :one
select count(*)
from posts
where @contract_id::dbid = any(posts.contract_ids)
and posts.deleted = false;

-- name: GetFeedEventsByIds :many
SELECT * FROM feed_events WHERE id = ANY(@ids::varchar(255)[]) AND deleted = false;

-- name: GetPostsByIdsPaginateBatch :batchmany
select posts.*
from posts
join unnest(@post_ids::varchar[]) with ordinality t(id, pos) using(id)
where not posts.deleted and t.pos > @cur_after_pos::int and t.pos < @cur_before_pos::int
order by t.pos asc;

-- name: GetEventByIdBatch :batchone
SELECT * FROM feed_events WHERE id = $1 AND deleted = false;

-- name: GetPostByIdBatch :batchone
SELECT * FROM posts WHERE id = $1 AND deleted = false;

-- name: GetPostByID :one
SELECT * FROM posts WHERE id = $1 AND deleted = false;

-- name: CreateFeedEvent :one
INSERT INTO feed_events (id, owner_id, action, data, event_time, event_ids, group_id, caption) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: IsFeedEventExistsForGroup :one
SELECT exists(
  SELECT 1 FROM feed_events WHERE deleted = false
  AND group_id = $1
);

-- name: UpdateFeedEventCaptionByGroup :one
UPDATE feed_events SET caption = (select caption from events where events.group_id = $1) WHERE group_id = $1 AND deleted = false returning *;

-- name: GetLastFeedEventForUser :one
select * from feed_events where deleted = false
    and owner_id = $1
    and action = any(@actions::action[])
    and event_time < $2
    order by event_time desc
    limit 1;

-- name: GetLastFeedEventForToken :one
select * from feed_events where deleted = false
    and owner_id = $1
    and action = any(@actions::action[])
    and data ->> 'token_id' = @token_id::varchar
    and event_time < $2
    order by event_time desc
    limit 1;

-- name: GetLastFeedEventForCollection :one
select * from feed_events where deleted = false
    and owner_id = $1
    and action = any(@actions::action[])
    and data ->> 'collection_id' = @collection_id::dbid
    and event_time < $2
    order by event_time desc
    limit 1;

-- name: GetUserIsBlockedFromFeed :one
select exists(select 1 from feed_blocklist where user_id = $1 and not deleted and active);

-- name: BlockUserFromFeed :exec
insert into feed_blocklist (id, user_id, reason, active) values ($1, $2, sqlc.narg('reason'), true)
on conflict(user_id) where not deleted do update set reason = coalesce(excluded.reason, feed_blocklist.reason), active = true, last_updated = now();

-- name: UnblockUserFromFeed :exec
update feed_blocklist set active = false where user_id = $1 and not deleted;

-- name: GetAdmireByAdmireID :one
SELECT * FROM admires WHERE id = $1 AND deleted = false;

-- name: GetAdmiresByAdmireIDs :many
SELECT * from admires WHERE id = ANY(@admire_ids::dbid[]) AND deleted = false;

-- name: GetAdmireByAdmireIDBatch :batchone
SELECT * FROM admires WHERE id = $1 AND deleted = false;

-- name: GetAdmiresByActorID :many
SELECT * FROM admires WHERE actor_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: GetAdmiresByActorIDBatch :batchmany
SELECT * FROM admires WHERE actor_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: PaginateAdmiresByFeedEventIDBatch :batchmany
SELECT * FROM admires WHERE feed_event_id = sqlc.arg('feed_event_id') AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id::dbid) AND (created_at, id) > (@cur_after_time, @cur_after_id::dbid)
    ORDER BY CASE WHEN sqlc.arg('paging_forward')::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT sqlc.arg('paging_forward')::bool THEN (created_at, id) END DESC
    LIMIT sqlc.arg('limit');

-- name: CountAdmiresByFeedEventIDBatch :batchone
SELECT count(*) FROM admires WHERE feed_event_id = $1 AND deleted = false;

-- name: PaginateAdmiresByPostIDBatch :batchmany
SELECT * FROM admires WHERE post_id = sqlc.arg('post_id') AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id::dbid) AND (created_at, id) > (@cur_after_time, @cur_after_id::dbid)
    ORDER BY CASE WHEN sqlc.arg('paging_forward')::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT sqlc.arg('paging_forward')::bool THEN (created_at, id) END DESC
    LIMIT sqlc.arg('limit');

-- name: CountAdmiresByPostIDBatch :batchone
SELECT count(*) FROM admires WHERE post_id = $1 AND deleted = false;

-- name: PaginateAdmiresByTokenIDBatch :batchmany
SELECT * FROM admires WHERE token_id = sqlc.arg('token_id') AND (not @only_for_actor::bool or actor_id = @actor_id) AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id::dbid) AND (created_at, id) > (@cur_after_time, @cur_after_id::dbid)
    ORDER BY CASE WHEN sqlc.arg('paging_forward')::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT sqlc.arg('paging_forward')::bool THEN (created_at, id) END DESC
    LIMIT sqlc.arg('limit');

-- name: CountAdmiresByTokenIDBatch :batchone
SELECT count(*) FROM admires WHERE token_id = $1 AND deleted = false;

-- name: PaginateAdmiresByCommentIDBatch :batchmany
select * from admires where comment_id = sqlc.arg('comment_id') and deleted = false
    and (created_at, id) < (@cur_before_time, @cur_before_id::dbid) and (created_at, id) > (@cur_after_time, @cur_after_id::dbid)
    order by case when sqlc.arg('paging_forward')::bool then (created_at, id) end asc,
             case when not sqlc.arg('paging_forward')::bool then (created_at, id) end desc
    limit sqlc.arg('limit');

-- name: CountAdmiresByCommentIDBatch :batchone
select count(*) from admires where comment_id = $1 and deleted = false;

-- name: GetCommentByCommentID :one
SELECT * FROM comments WHERE id = $1 AND deleted = false;

-- name: GetCommentsByCommentIDs :many
SELECT * from comments WHERE id = ANY(@comment_ids::dbid[]) AND deleted = false;

-- name: GetCommentByCommentIDBatch :batchone
SELECT * FROM comments WHERE id = $1 AND deleted = false; 

-- name: PaginateCommentsByFeedEventIDBatch :batchmany
SELECT * FROM comments WHERE feed_event_id = sqlc.arg('feed_event_id') AND reply_to is null AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id::dbid)
    AND (created_at, id) > (@cur_after_time, @cur_after_id::dbid)
    ORDER BY CASE WHEN sqlc.arg('paging_forward')::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT sqlc.arg('paging_forward')::bool THEN (created_at, id) END DESC
    LIMIT sqlc.arg('limit');

-- name: CountCommentsByFeedEventIDBatch :batchone
SELECT count(*) FROM comments WHERE feed_event_id = sqlc.arg('feed_event_id') AND reply_to is null AND deleted = false;

-- name: CountCommentsAndRepliesByFeedEventID :one
SELECT count(*) FROM comments WHERE feed_event_id = sqlc.arg('feed_event_id') AND deleted = false;

-- name: PaginateCommentsByPostIDBatch :batchmany
SELECT * FROM comments WHERE post_id = sqlc.arg('post_id') AND reply_to is null AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id::dbid)
    AND (created_at, id) > (@cur_after_time, @cur_after_id::dbid)
    ORDER BY CASE WHEN sqlc.arg('paging_forward')::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT sqlc.arg('paging_forward')::bool THEN (created_at, id) END DESC
    LIMIT sqlc.arg('limit');

-- name: CountCommentsByPostIDBatch :batchone
SELECT count(*) FROM comments WHERE post_id = sqlc.arg('post_id') AND reply_to is null AND deleted = false;

-- name: CountCommentsAndRepliesByPostID :one
SELECT count(*) FROM comments WHERE post_id = sqlc.arg('post_id') AND deleted = false;

-- name: PaginateRepliesByCommentIDBatch :batchmany
SELECT * FROM comments c 
WHERE 
    CASE 
        WHEN (SELECT reply_to FROM comments cc WHERE cc.id = sqlc.arg('comment_id')) IS NULL 
        THEN c.top_level_comment_id = sqlc.arg('comment_id') 
        ELSE c.reply_to = sqlc.arg('comment_id') 
    END
    AND c.deleted = false
    AND (c.created_at, c.id) < (@cur_before_time, @cur_before_id::dbid)
    AND (c.created_at, c.id) > (@cur_after_time, @cur_after_id::dbid)
ORDER BY 
    CASE 
        WHEN sqlc.arg('paging_forward')::bool THEN (c.created_at, c.id) 
    END ASC,
    CASE 
        WHEN NOT sqlc.arg('paging_forward')::bool THEN (c.created_at, c.id) 
    END DESC
LIMIT sqlc.arg('limit');

-- name: CountRepliesByCommentIDBatch :batchone
SELECT count(*) FROM comments c 
WHERE 
    CASE 
        WHEN (SELECT reply_to FROM comments cc WHERE cc.id = sqlc.arg('comment_id')) IS NULL 
        THEN c.top_level_comment_id = sqlc.arg('comment_id') 
        ELSE c.reply_to = sqlc.arg('comment_id') 
    END
    AND c.deleted = false;

-- name: GetUserNotifications :many
SELECT * FROM notifications WHERE owner_id = $1 AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id::dbid)
    AND (created_at, id) > (@cur_after_time, @cur_after_id::dbid)
    ORDER BY CASE WHEN @paging_forward::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT @paging_forward::bool THEN (created_at, id) END DESC
    LIMIT $2;

-- name: GetUserUnseenNotifications :many
SELECT * FROM notifications WHERE owner_id = $1 AND deleted = false AND seen = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id::dbid)
    AND (created_at, id) > (@cur_after_time, @cur_after_id::dbid)
    ORDER BY CASE WHEN @paging_forward::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT @paging_forward::bool THEN (created_at, id) END DESC
    LIMIT $2;

-- name: GetRecentUnseenNotifications :many
SELECT * FROM notifications WHERE owner_id = @owner_id AND deleted = false AND seen = false and created_at > @created_after order by created_at desc limit @lim;

-- name: GetUserNotificationsBatch :batchmany
SELECT * FROM notifications WHERE owner_id = sqlc.arg('owner_id') AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id::dbid)
    AND (created_at, id) > (@cur_after_time, @cur_after_id::dbid)
    ORDER BY CASE WHEN sqlc.arg('paging_forward')::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT sqlc.arg('paging_forward')::bool THEN (created_at, id) END DESC
    LIMIT sqlc.arg('limit');

-- name: CountUserNotifications :one
SELECT count(*) FROM notifications WHERE owner_id = $1 AND deleted = false;

-- name: CountUserUnseenNotifications :one
SELECT count(*) FROM notifications WHERE owner_id = $1 AND deleted = false AND seen = false;

-- name: GetNotificationByID :one
SELECT * FROM notifications WHERE id = $1 AND deleted = false;

-- name: GetNotificationByIDBatch :batchone
SELECT * FROM notifications WHERE id = $1 AND deleted = false;

-- name: GetMostRecentNotificationByOwnerIDForAction :one
select * from notifications
    where owner_id = $1
    and action = $2
    and deleted = false
    and (not @only_for_feed_event::bool or feed_event_id = $3)
    and (not @only_for_post::bool or post_id = $4)
    and (not @only_for_comment::bool or comment_id = $5)
    order by created_at desc
    limit 1;

-- name: GetMostRecentNotificationByOwnerIDTokenIDForAction :one
select * from notifications
    where owner_id = $1
    and token_id = $2
    and action = $3
    and deleted = false
    and (not @only_for_feed_event::bool or feed_event_id = $4)
    and (not @only_for_post::bool or post_id = $5)
    order by created_at desc
    limit 1;

-- name: GetNotificationsByOwnerIDForActionAfter :many
SELECT * FROM notifications
    WHERE owner_id = $1 AND action = $2 AND deleted = false AND created_at > @created_after
    ORDER BY created_at DESC;

-- name: CreateAdmireNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids, feed_event_id, post_id, token_id) VALUES ($1, $2, $3, $4, $5, sqlc.narg('feed_event')::text, sqlc.narg('post')::text, sqlc.narg('token')::text) RETURNING *;

-- name: CreateCommentNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids, feed_event_id, post_id, comment_id) VALUES ($1, $2, $3, $4, $5, sqlc.narg('feed_event')::text, sqlc.narg('post')::text, $6) RETURNING *;

-- name: CreateCommunityNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids, feed_event_id, post_id, comment_id, community_id, mention_id) VALUES ($1, $2, $3, $4, $5, sqlc.narg('feed_event')::text, sqlc.narg('post')::text, sqlc.narg('comment')::text, $6, $7) RETURNING *;

-- name: CreateUserPostedYourWorkNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids, post_id, community_id) VALUES ($1, $2, $3, $4, $5, sqlc.narg('post')::text, $6) RETURNING *;

-- name: CountFollowersByUserID :one
SELECT count(*) FROM follows WHERE followee = $1 AND deleted = false;

-- later on, we might want to add a "global" column to notifications or even an enum column like "match" to determine how largely consumed
-- notifications will get searched for for a given user. For example, global notifications will always return for a user and follower notifications will
-- perform the check to see if the user follows the owner of the notification. Where this breaks is how we handle "seen" notifications. Since there is 1:1 notifications to users
-- right now, we can't have a "seen" field on the notification itself. We would have to move seen out into a separate table.
-- name: CreateAnnouncementNotifications :many
WITH 
id_with_row_number AS (
    SELECT unnest(@ids::varchar(255)[]) AS id, row_number() OVER (ORDER BY unnest(@ids::varchar(255)[])) AS rn
),
user_with_row_number AS (
    SELECT id AS user_id, row_number() OVER () AS rn
    FROM users
    WHERE deleted = false AND universal = false
)
INSERT INTO notifications (id, owner_id, action, data, event_ids)
SELECT 
    i.id, 
    u.user_id, 
    $1, 
    $2, 
    $3
FROM 
    id_with_row_number i
JOIN 
    user_with_row_number u ON i.rn = u.rn
WHERE NOT EXISTS (
    SELECT 1
    FROM notifications n
    WHERE n.owner_id = u.user_id 
    AND n.data ->> 'internal_id' = sqlc.arg('internal')::varchar
)
RETURNING *;

-- name: CountAllUsers :one
SELECT count(*) FROM users WHERE deleted = false and universal = false;

-- name: CreateSimpleNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: CreateTokenNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids, token_id, amount) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: CreateMentionUserNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids, feed_event_id, post_id, comment_id, mention_id) VALUES ($1, $2, $3, $4, $5, sqlc.narg('feed_event')::text, sqlc.narg('post')::text, sqlc.narg('comment')::text, $6) RETURNING *;

-- name: CreateViewGalleryNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids, gallery_id) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: CreatePostNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids, post_id) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: UpdateNotification :exec
UPDATE notifications SET data = $2, event_ids = event_ids || $3, amount = $4, last_updated = now(), seen = false WHERE id = $1 AND deleted = false AND NOT amount = $4;

-- name: UpdateNotificationSettingsByID :exec
UPDATE users SET notification_settings = $2 WHERE id = $1;

-- name: ClearNotificationsForUser :many
UPDATE notifications SET seen = true WHERE owner_id = $1 AND seen = false RETURNING *;

-- name: PaginateInteractionsByFeedEventIDBatch :batchmany
SELECT interactions.created_At, interactions.id, interactions.tag FROM (
    SELECT t.created_at, t.id, sqlc.arg('admire_tag')::int as tag FROM admires t WHERE sqlc.arg('admire_tag') != 0 AND t.feed_event_id = sqlc.arg('feed_event_id') AND t.deleted = false
        AND (sqlc.arg('admire_tag'), t.created_at, t.id) < (sqlc.arg('cur_before_tag')::int, @cur_before_time, @cur_before_id::dbid) AND (sqlc.arg('admire_tag'), t.created_at, t.id) > (sqlc.arg('cur_after_tag')::int, @cur_after_time, @cur_after_id::dbid)
                                                                    UNION
    SELECT t.created_at, t.id, sqlc.arg('comment_tag')::int as tag FROM comments t WHERE sqlc.arg('comment_tag') != 0 AND t.feed_event_id = sqlc.arg('feed_event_id') AND t.reply_to is null AND t.deleted = false
        AND (sqlc.arg('comment_tag'), t.created_at, t.id) < (sqlc.arg('cur_before_tag')::int, @cur_before_time, @cur_before_id::dbid) AND (sqlc.arg('comment_tag'), t.created_at, t.id) > (sqlc.arg('cur_after_tag')::int, @cur_after_time, @cur_after_id::dbid)
) as interactions

ORDER BY CASE WHEN sqlc.arg('paging_forward')::bool THEN (tag, created_at, id) END ASC,
         CASE WHEN NOT sqlc.arg('paging_forward')::bool THEN (tag, created_at, id) END DESC
LIMIT sqlc.arg('limit');

-- name: CountInteractionsByFeedEventIDBatch :batchmany
SELECT count(*), sqlc.arg('admire_tag')::int as tag FROM admires t WHERE sqlc.arg('admire_tag') != 0 AND t.feed_event_id = sqlc.arg('feed_event_id') AND t.deleted = false
                                                        UNION
SELECT count(*), sqlc.arg('comment_tag')::int as tag FROM comments t WHERE sqlc.arg('comment_tag') != 0 AND t.feed_event_id = sqlc.arg('feed_event_id') AND t.reply_to is null AND t.deleted = false;

-- name: PaginateInteractionsByPostIDBatch :batchmany
SELECT interactions.created_At, interactions.id, interactions.tag FROM (
    SELECT t.created_at, t.id, sqlc.arg('admire_tag')::int as tag FROM admires t WHERE sqlc.arg('admire_tag') != 0 AND t.post_id = sqlc.arg('post_id') AND t.deleted = false
        AND (sqlc.arg('admire_tag'), t.created_at, t.id) < (sqlc.arg('cur_before_tag')::int, @cur_before_time, @cur_before_id::dbid) AND (sqlc.arg('admire_tag'), t.created_at, t.id) > (sqlc.arg('cur_after_tag')::int, @cur_after_time, @cur_after_id::dbid)
                                                                    UNION
    SELECT t.created_at, t.id, sqlc.arg('comment_tag')::int as tag FROM comments t WHERE sqlc.arg('comment_tag') != 0 AND t.post_id = sqlc.arg('post_id') AND t.reply_to is null AND t.deleted = false
        AND (sqlc.arg('comment_tag'), t.created_at, t.id) < (sqlc.arg('cur_before_tag')::int, @cur_before_time, @cur_before_id::dbid) AND (sqlc.arg('comment_tag'), t.created_at, t.id) > (sqlc.arg('cur_after_tag')::int, @cur_after_time, @cur_after_id::dbid)
) as interactions

ORDER BY CASE WHEN sqlc.arg('paging_forward')::bool THEN (tag, created_at, id) END ASC,
         CASE WHEN NOT sqlc.arg('paging_forward')::bool THEN (tag, created_at, id) END DESC
LIMIT sqlc.arg('limit');

-- name: CountInteractionsByPostIDBatch :batchmany
SELECT count(*), sqlc.arg('admire_tag')::int as tag FROM admires t WHERE sqlc.arg('admire_tag') != 0 AND t.post_id = sqlc.arg('post_id') AND t.deleted = false
                                                        UNION
SELECT count(*), sqlc.arg('comment_tag')::int as tag FROM comments t WHERE sqlc.arg('comment_tag') != 0 AND t.post_id = sqlc.arg('post_id') AND t.reply_to is null AND t.deleted = false;

-- name: GetAdmireByActorIDAndFeedEventID :batchone
SELECT * FROM admires WHERE actor_id = $1 AND feed_event_id = $2 AND deleted = false;

-- name: GetAdmireByActorIDAndPostID :batchone
SELECT * FROM admires WHERE actor_id = $1 AND post_id = $2 AND deleted = false;

-- name: GetAdmireByActorIDAndTokenID :batchone
SELECT * FROM admires WHERE actor_id = $1 AND token_id = $2 AND deleted = false;

-- name: GetAdmireByActorIDAndCommentID :batchone
SELECT * FROM admires WHERE actor_id = $1 AND comment_id = $2 AND deleted = false;

-- name: InsertPost :one
insert into posts(id, token_ids, contract_ids, actor_id, caption, user_mint_url, is_first_post, created_at)
values ($1, $2, $3, $4, $5, $6, not exists(select 1 from posts where posts.created_at < now() and posts.actor_id = $4::varchar limit 1), now())
on conflict (actor_id, is_first_post) where is_first_post do update set is_first_post = false
returning id;

-- name: DeletePostByID :exec
update posts set deleted = true where id = $1;

-- for some reason this query will not allow me to use @tags for $1
-- name: GetUsersWithEmailNotificationsOnForEmailType :many
select u.* from pii.user_view u
    left join user_roles r on r.user_id = u.id and r.role = 'EMAIL_TESTER' and r.deleted = false
    where (u.email_unsubscriptions->>'all' = 'false' or u.email_unsubscriptions->>'all' is null)
    and (u.email_unsubscriptions->>sqlc.arg(email_unsubscription)::varchar = 'false' or u.email_unsubscriptions->>sqlc.arg(email_unsubscription)::varchar is null)
    and u.deleted = false and u.pii_verified_email_address is not null
    and (u.created_at, u.id) < (@cur_before_time, @cur_before_id::dbid)
    and (u.created_at, u.id) > (@cur_after_time, @cur_after_id::dbid)
    and (@email_testers_only::bool = false or r.user_id is not null)
    order by case when @paging_forward::bool then (u.created_at, u.id) end asc,
             case when not @paging_forward::bool then (u.created_at, u.id) end desc
    limit $1;

-- name: GetUsersWithRolePaginate :many
select u.* from users u, user_roles ur where u.deleted = false and ur.deleted = false
    and u.id = ur.user_id and ur.role = @role
    and (u.username_idempotent, u.id) < (@cur_before_key::varchar, @cur_before_id::dbid)
    and (u.username_idempotent, u.id) > (@cur_after_key::varchar, @cur_after_id::dbid)
    order by case when @paging_forward::bool then (u.username_idempotent, u.id) end asc,
             case when not @paging_forward::bool then (u.username_idempotent, u.id) end desc
    limit $1;

-- name: GetUsersByPositionPaginateBatch :batchmany
select u.*
from users u
join unnest(@user_ids::varchar[]) with ordinality t(id, pos) using(id)
where not u.deleted and not u.universal and t.pos > @cur_after_pos::int and t.pos < @cur_before_pos::int
order by t.pos asc;

-- name: GetUsersByPositionPersonalizedBatch :batchmany
select u.*
from users u
join unnest(@user_ids::varchar[]) with ordinality t(id, pos) using(id)
left join follows on follows.follower = @viewer_id and follows.followee = u.id
where not u.deleted and not u.universal and follows.id is null
order by t.pos
limit 100;

-- name: UpdateUserVerifiedEmail :exec
insert into pii.for_users (user_id, pii_unverified_email_address, pii_verified_email_address) values (@user_id, null, @email_address)
    on conflict (user_id) do update
        set pii_verified_email_address = excluded.pii_verified_email_address,
            pii_unverified_email_address = excluded.pii_unverified_email_address;

-- name: UpdateUserUnverifiedEmail :exec
insert into pii.for_users (user_id, pii_unverified_email_address, pii_verified_email_address) values (@user_id, @email_address, null)
    on conflict (user_id) do update
        set pii_unverified_email_address = excluded.pii_unverified_email_address,
            pii_verified_email_address = excluded.pii_verified_email_address;

-- name: UpdateUserEmailUnsubscriptions :exec
UPDATE users SET email_unsubscriptions = $2 WHERE id = $1;

-- name: UpdateUserPrimaryWallet :exec
update users set primary_wallet_id = @wallet_id from wallets
    where users.id = @user_id and wallets.id = @wallet_id
    and wallets.id = any(users.wallets) and wallets.deleted = false;

-- name: GetUsersByChainAddresses :many
select users.*,wallets.address from users, wallets where wallets.address = ANY(@addresses::varchar[]) AND wallets.l1_chain = @l1_chain AND ARRAY[wallets.id] <@ users.wallets AND users.deleted = false AND wallets.deleted = false;

-- name: GetUsersByFarcasterIDs :many
select users.*
from pii.for_users join users on for_users.user_id = users.id
where ((pii_socials -> 'Farcaster'::text) ->> 'id'::text) = any(@fids::varchar[]) and not users.deleted;

-- name: GetFeedEventByID :one
SELECT * FROM feed_events WHERE id = $1 AND deleted = false;

-- name: AddUserRoles :exec
insert into user_roles (id, user_id, role, created_at, last_updated)
select unnest(@ids::varchar[]), $1, unnest(@roles::varchar[]), now(), now()
on conflict (user_id, role) do update set deleted = false, last_updated = now();

-- name: DeleteUserRoles :exec
update user_roles set deleted = true, last_updated = now() where user_id = $1 and role = any(@roles::role[]);

-- name: GetUserRolesByUserId :many
with membership_roles(role) as (
    select (case when exists(
        select 1
        from tokens
        join token_definitions on tokens.token_definition_id = token_definitions.id
        where owner_user_id = @user_id
            and token_definitions.chain = @chain
            and token_definitions.contract_address = @membership_address
            and token_definitions.token_id = any(@membership_token_ids::varchar[])
            and tokens.displayable
            and not tokens.deleted
            and not token_definitions.deleted
    ) then @granted_membership_role else null end)::varchar
)
select role from user_roles where user_id = @user_id and deleted = false
union
select role from membership_roles where role is not null;

-- name: RedeemMerch :one
update merch set redeemed = true, token_id = @token_hex, last_updated = now() where id = (select m.id from merch m where m.object_type = @object_type and m.token_id is null and m.redeemed = false and m.deleted = false order by m.id limit 1) and token_id is null and redeemed = false returning discount_code;

-- name: GetMerchDiscountCodeByTokenID :one
select discount_code from merch where token_id = @token_hex and redeemed = true and deleted = false;

-- name: UpdateGalleryHidden :one
update galleries set hidden = @hidden, last_updated = now() where id = @id and deleted = false returning *;

-- name: UpdateGalleryPositions :exec
with updates as (
    select unnest(@gallery_ids::text[]) as id, unnest(@positions::text[]) as position
)
update galleries g set position = updates.position, last_updated = now() from updates where g.id = updates.id and deleted = false and g.owner_user_id = @owner_user_id;

-- name: UserHasDuplicateGalleryPositions :one
select exists(select position,count(*) from galleries where owner_user_id = $1 and deleted = false group by position having count(*) > 1);

-- name: UpdateGalleryInfo :exec
update galleries set name = case when @name_set::bool then @name else name end, description = case when @description_set::bool then @description else description end, last_updated = now() where id = @id and deleted = false;

-- name: UpdateGalleryCollections :exec
update galleries set collections = @collections, last_updated = now() where galleries.id = @gallery_id and galleries.deleted = false and (select count(*) from collections c where c.id = any(@collections) and c.gallery_id = @gallery_id and c.deleted = false) = cardinality(@collections);

-- name: UpdateUserFeaturedGallery :exec
update users set featured_gallery = @gallery_id::dbid, last_updated = now() from galleries where users.id = @user_id and galleries.id = @gallery_id and galleries.owner_user_id = @user_id and galleries.deleted = false;

-- name: GetGalleryTokenMediasByGalleryIDBatch :batchmany
select tm.*
from galleries g, collections c, tokens t, token_medias tm, token_definitions td
where
	g.id = $1
	and c.id = any(g.collections)
	and t.id = any(c.nfts)
    and t.owner_user_id = g.owner_user_id
    and t.displayable
    and t.token_definition_id = td.id
    and td.token_media_id = tm.id
    and not td.deleted
	and not g.deleted
	and not c.deleted
	and not t.deleted
	and not tm.deleted
	and (
		tm.media->>'thumbnail_url' is not null
		or ((tm.media->>'media_type' = 'image' or tm.media->>'media_type' = 'gif') and tm.media->>'media_url' is not null)
	)
order by array_position(g.collections, c.id) , array_position(c.nfts, t.id)
limit 4;

-- name: DeleteCollections :exec
update collections set deleted = true, last_updated = now() where id = any(@ids::varchar[]);

-- name: UpdateCollectionsInfo :exec
with updates as (
    select unnest(@ids::varchar[]) as id, unnest(@names::varchar[]) as name, unnest(@collectors_notes::varchar[]) as collectors_note, unnest(@layouts::jsonb[]) as layout, unnest(@token_settings::jsonb[]) as token_settings, unnest(@hidden::bool[]) as hidden
)
update collections c set collectors_note = updates.collectors_note, layout = updates.layout, token_settings = updates.token_settings, hidden = updates.hidden, name = updates.name, last_updated = now(), version = 1 from updates where c.id = updates.id and c.deleted = false;

-- name: GetCollectionTokensByCollectionID :one
select nfts from collections where id = $1 and deleted = false;

-- name: UpdateCollectionTokens :exec
update collections set nfts = @nfts, last_updated = now() where id = @id and deleted = false;

-- name: CreateCollection :one
insert into collections (id, version, name, collectors_note, owner_user_id, gallery_id, layout, nfts, hidden, token_settings, created_at, last_updated) values (@id, 1, @name, @collectors_note, @owner_user_id, @gallery_id, @layout, @nfts, @hidden, @token_settings, now(), now()) returning id;

-- name: GetGalleryIDByCollectionID :one
select gallery_id from collections where id = $1 and deleted = false;

-- name: GetAllTimeTrendingUserIDs :many
select users.id
from events, galleries, users
left join legacy_views on users.id = legacy_views.user_id and legacy_views.deleted = false
where action = 'ViewedGallery'
  and events.gallery_id = galleries.id
  and users.id = galleries.owner_user_id
  and galleries.deleted = false
  and users.deleted = false
group by users.id
order by row_number() over(order by count(events.id) + coalesce(max(legacy_views.view_count), 0) desc, max(users.created_at) desc) asc
limit $1;

-- name: GetWindowedTrendingUserIDs :many
with viewers as (
  select gallery_id, count(distinct coalesce(actor_id, external_id)) viewer_count
  from events
  where action = 'ViewedGallery' and events.created_at >= @window_end
  group by gallery_id
),
edit_events as (
  select actor_id
  from events
  where action in (
    'CollectionCreated',
    'CollectorsNoteAddedToCollection',
    'CollectorsNoteAddedToToken',
    'TokensAddedToCollection',
    'GalleryInfoUpdated'
  ) and created_at >= @window_end
  group by actor_id
)
select users.id
from viewers, galleries, users, edit_events
where viewers.gallery_id = galleries.id
	and galleries.owner_user_id = users.id
	and users.deleted = false
	and galleries.deleted = false
  and users.id = edit_events.actor_id
group by users.id
order by row_number() over(order by sum(viewers.viewer_count) desc, max(users.created_at) desc) asc
limit $1;

-- name: GetUserExperiencesByUserID :one
select user_experiences from users where id = $1;

-- name: UpdateUserExperience :exec
update users set user_experiences = user_experiences || @experience where id = @user_id;

-- name: GetTrendingUsersByIDs :many
select users.* from users join unnest(@user_ids::varchar[]) with ordinality t(id, pos) using (id) where deleted = false order by t.pos asc;

-- name: UpdateCollectionGallery :exec
update collections set gallery_id = @gallery_id, last_updated = now() where id = @id and deleted = false;

-- name: AddCollectionToGallery :exec
update galleries set collections = array_append(collections, @collection_id), last_updated = now() where id = @gallery_id and deleted = false;

-- name: RemoveCollectionFromGallery :exec
update galleries set collections = array_remove(collections, @collection_id), last_updated = now() where id = @gallery_id and deleted = false;

-- name: UserOwnsGallery :one
select exists(select 1 from galleries where id = $1 and owner_user_id = $2 and deleted = false);

-- name: UserOwnsCollection :one
select exists(select 1 from collections where id = $1 and owner_user_id = $2 and deleted = false);

-- name: GetSocialAuthByUserID :one
select * from pii.socials_auth where user_id = $1 and provider = $2 and deleted = false;

-- name: UpsertSocialOAuth :exec
insert into pii.socials_auth (id, user_id, provider, access_token, refresh_token) values (@id, @user_id, @provider, @access_token, @refresh_token) on conflict (user_id, provider) where deleted = false do update set access_token = @access_token, refresh_token = @refresh_token, last_updated = now();

-- name: AddSocialToUser :exec
insert into pii.for_users (user_id, pii_socials) values (@user_id, @socials) on conflict (user_id) where deleted = false do update set pii_socials = for_users.pii_socials || @socials;

-- name: RemoveSocialFromUser :exec
update pii.for_users set pii_socials = pii_socials - @social::varchar where user_id = @user_id;

-- name: GetSocialsByUserID :one
select pii_socials from pii.user_view where id = $1;

-- name: UpdateUserSocials :exec
update pii.for_users set pii_socials = @socials where user_id = @user_id;

-- name: UpdateEventCaptionByGroup :exec
update events set caption = @caption where group_id = @group_id and deleted = false;

-- name: GetFarcasterConnections :many
with farcaster         as ( select unnest(@fids::varchar[]) fid ),
	 farcaster_gallery as ( select * from pii.for_users p join farcaster on p.pii_socials->'Farcaster'->>'id' = farcaster.fid ),
	 not_followed      as ( select * from farcaster_gallery fg left join follows f on f.follower = @user_id and f.followee = fg.user_id where f.id is null ),
	 farcaster_users   as ( select u.* from users u join not_followed on u.id = not_followed.user_id and not u.deleted and not u.universal ),
	 ordering          as ( select f.id
	                            , rank() over (order by sum(cardinality(c.nfts)) desc nulls last) display_rank
	                            , rank() over (order by count(p.id) desc nulls last) post_rank
	 						from farcaster_users f
	 						left join collections c on f.id = c.owner_user_id and not c.deleted
	 						left join posts p on f.id = p.actor_id and not p.deleted
	 						group by f.id )
select sqlc.embed(users)
from farcaster_users users
left join ordering using(id)
order by ((ordering.display_rank + ordering.post_rank) / 2) asc nulls last
limit 200;

-- this query will take in enoug info to create a sort of fake table of social accounts matching them up to users in gallery with twitter connected.
-- it will also go and search for whether the specified user follows any of the users returned
-- name: GetSocialConnectionsPaginate :many
select s.*, user_view.id as user_id, user_view.created_at as user_created_at, (f.id is not null)::bool as already_following
from (select unnest(@social_ids::varchar[]) as social_id, unnest(@social_usernames::varchar[]) as social_username, unnest(@social_displaynames::varchar[]) as social_displayname, unnest(@social_profile_images::varchar[]) as social_profile_image) as s
    inner join pii.user_view on user_view.pii_socials->sqlc.arg('social')::text->>'id'::varchar = s.social_id and user_view.deleted = false
    left outer join follows f on f.follower = @user_id and f.followee = user_view.id and f.deleted = false
where case when @only_unfollowing::bool then f.id is null else true end
    and (f.id is not null,user_view.created_at,user_view.id) < (@cur_before_following::bool, @cur_before_time::timestamptz, @cur_before_id::dbid)
    and (f.id is not null,user_view.created_at,user_view.id) > (@cur_after_following::bool, @cur_after_time::timestamptz, @cur_after_id::dbid)
order by case when @paging_forward::bool then (f.id is not null,user_view.created_at,user_view.id) end asc,
    case when not @paging_forward::bool then (f.id is not null,user_view.created_at,user_view.id) end desc
limit $1;

-- name: GetSocialConnections :many
select s.*, user_view.id as user_id, user_view.created_at as user_created_at, (f.id is not null)::bool as already_following
from (select unnest(@social_ids::varchar[]) as social_id, unnest(@social_usernames::varchar[]) as social_username, unnest(@social_displaynames::varchar[]) as social_displayname, unnest(@social_profile_images::varchar[]) as social_profile_image) as s
    inner join pii.user_view on user_view.pii_socials->sqlc.arg('social')::text->>'id'::varchar = s.social_id and user_view.deleted = false
    left outer join follows f on f.follower = @user_id and f.followee = user_view.id and f.deleted = false
where case when @only_unfollowing::bool then f.id is null else true end
order by (f.id is not null,user_view.created_at,user_view.id);

-- name: CountSocialConnections :one
select count(*)
from (select unnest(@social_ids::varchar[]) as social_id) as s
    inner join pii.user_view on user_view.pii_socials->sqlc.arg('social')::text->>'id'::varchar = s.social_id and user_view.deleted = false
    left outer join follows f on f.follower = @user_id and f.followee = user_view.id and f.deleted = false
where case when @only_unfollowing::bool then f.id is null else true end;

-- name: AddManyFollows :many
insert into follows (id, follower, followee, deleted) select unnest(@ids::varchar[]), @follower, unnest(@followees::varchar[]), false on conflict (follower, followee) where deleted = false do update set deleted = false, last_updated = now() returning last_updated > created_at;

-- name: UsersFollowUser :many
select (follows.id is not null)::bool
from (
    select unnest(@followed_ids::varchar[]) as id,
    generate_subscripts(@followed_ids::varchar[], 1) as index
    ) user_ids
left join follows on follows.follower = user_ids.id and followee = $1 and not deleted
order by user_ids.index;

-- name: GetSharedFollowersBatchPaginate :batchmany
select sqlc.embed(users), a.created_at followed_on
from users, follows a, follows b
where a.follower = @follower
	and a.followee = b.follower
	and b.followee = @followee
	and users.id = b.follower
	and a.deleted = false
	and b.deleted = false
	and users.deleted = false
  and (a.created_at, users.id) > (@cur_before_time, @cur_before_id::dbid)
  and (a.created_at, users.id) < (@cur_after_time, @cur_after_id::dbid)
order by case when sqlc.arg('paging_forward')::bool then (a.created_at, users.id) end desc,
        case when not sqlc.arg('paging_forward')::bool then (a.created_at, users.id) end asc
limit sqlc.arg('limit');

-- name: CountSharedFollows :one
select count(*)
from users, follows a, follows b
where a.follower = @follower
	and a.followee = b.follower
	and b.followee = @followee
	and users.id = b.follower
	and a.deleted = false
	and b.deleted = false
	and users.deleted = false;

-- name: GetSharedCommunitiesBatchPaginate :batchmany
select sqlc.embed(communities), a.displayed as displayed_by_user_a, b.displayed as displayed_by_user_b, a.owned_count
from owned_communities a, owned_communities b, communities
left join contracts on communities.contract_id = contracts.id
left join marketplace_contracts on communities.contract_id = marketplace_contracts.contract_id
where a.user_id = @user_a_id
  and b.user_id = @user_b_id
  and a.community_id = b.community_id
  and a.community_id = communities.id
  and marketplace_contracts.contract_id is null
  and communities.name != ''
  and communities.name != 'Unidentified contract'
  and (contracts.is_provider_marked_spam is null or contracts.is_provider_marked_spam = false)
  and (
    a.displayed,
    b.displayed,
    a.owned_count,
    communities.id
  ) > (
    @cur_before_displayed_by_user_a,
    @cur_before_displayed_by_user_b,
    @cur_before_owned_count::int,
    @cur_before_contract_id::dbid
  )
  and (
    a.displayed,
    b.displayed,
    a.owned_count,
    communities.id
  ) < (
    @cur_after_displayed_by_user_a,
    @cur_after_displayed_by_user_b,
    @cur_after_owned_count::int,
    @cur_after_contract_id::dbid
  )
order by case when sqlc.arg('paging_forward')::bool then (a.displayed, b.displayed, a.owned_count, communities.id) end desc,
        case when not sqlc.arg('paging_forward')::bool then (a.displayed, b.displayed, a.owned_count, communities.id) end asc
limit sqlc.arg('limit');

-- name: GetCreatedContractsBatchPaginate :batchmany
select contracts.*
from contracts
    join contract_creators on contracts.id = contract_creators.contract_id and contract_creators.creator_user_id = @user_id
where (@include_all_chains::bool or contracts.chain = any(string_to_array(@chains, ',')::int[]))
  and (contracts.created_at, contracts.id) < (@cur_before_time, @cur_before_id::dbid)
  and (contracts.created_at, contracts.id) > ( @cur_after_time, @cur_after_id::dbid)
order by case when sqlc.arg('paging_forward')::bool then (contracts.created_at, contracts.id) end asc,
        case when not sqlc.arg('paging_forward')::bool then (contracts.created_at, contracts.id) end desc
limit sqlc.arg('limit');

-- name: CountSharedCommunities :one
select count(*)
from owned_communities a, owned_communities b, communities
left join contracts on communities.contract_id = contracts.id
left join marketplace_contracts on communities.contract_id = marketplace_contracts.contract_id
where a.user_id = @user_a_id
  and b.user_id = @user_b_id
  and a.community_id = b.community_id
  and a.community_id = communities.id
  and marketplace_contracts.contract_id is null
  and communities.name != ''
  and communities.name != 'Unidentified contract'
  and (contracts.is_provider_marked_spam is null or contracts.is_provider_marked_spam = false);

-- name: AddPiiAccountCreationInfo :exec
insert into pii.account_creation_info (user_id, ip_address, created_at) values (@user_id, @ip_address, now())
  on conflict do nothing;

-- name: GetChildContractsByParentIDBatchPaginate :batchmany
select c.*
from contracts c
where c.parent_id = @parent_id
  and c.deleted = false
  and (c.created_at, c.id) < (@cur_before_time, @cur_before_id::dbid)
  and (c.created_at, c.id) > (@cur_after_time, @cur_after_id::dbid)
order by case when sqlc.arg('paging_forward')::bool then (c.created_at, c.id) end asc,
        case when not sqlc.arg('paging_forward')::bool then (c.created_at, c.id) end desc
limit sqlc.arg('limit');

-- name: GetUserByWalletID :one
select * from users where array[@wallet::varchar]::varchar[] <@ wallets and deleted = false;

-- name: DeleteUserByID :exec
update users set deleted = true where id = $1;

-- name: InsertWallet :exec
with new_wallet as (insert into wallets(id, address, chain, l1_chain, wallet_type) values ($1, $2, $3, $4, $5) returning id)
update users set
    primary_wallet_id = coalesce(users.primary_wallet_id, new_wallet.id),
    wallets = array_append(users.wallets, new_wallet.id)
from new_wallet
where users.id = @user_id and not users.deleted;

-- name: DeleteWalletByID :exec
update wallets set deleted = true, last_updated = now() where id = $1;

-- name: InsertUser :one
insert into users (id, username, username_idempotent, bio, universal, email_unsubscriptions) values ($1, $2, $3, $4, $5, $6) returning id;

-- name: InsertTokenPipelineResults :one
with insert_job(id) as (
    insert into token_processing_jobs (id, token_properties, pipeline_metadata, processing_cause, processor_version)
    values (@processing_job_id, @token_properties, @pipeline_metadata, @processing_cause, @processor_version)
    returning id
)
, set_conditionally_current_media_to_inactive(last_updated) as (
    insert into token_medias (id, media, processing_job_id, active, created_at, last_updated)
    (
        select @retiring_media_id, media, processing_job_id, false, created_at, now()
        from token_medias
        where id = (select token_media_id from token_definitions td where (td.chain, td.contract_address, td.token_id) = (@chain, @contract_address::address, @token_id::hextokenid) and not deleted)
        and not deleted
        and @new_media_is_active::bool
    )
    returning last_updated
)
, insert_new_media as (
    insert into token_medias (id, chain, contract_address, token_id, media, processing_job_id, active, created_at, last_updated)
    values (@new_media_id, @chain, @contract_address, @decimal_token_id, @new_media::jsonb, (select id from insert_job), @new_media_is_active,
        -- Using timestamps generated from set_conditionally_current_media_to_inactive ensures that the new record is only inserted after the current media is moved
        (select coalesce((select last_updated from set_conditionally_current_media_to_inactive), now())),
        (select coalesce((select last_updated from set_conditionally_current_media_to_inactive), now()))
    )
    returning *
)
, update_token_definition as (
    update token_definitions
    set metadata = @new_metadata::jsonb,
        name = @new_name,
        description = @new_description,
        last_updated = (select last_updated from insert_new_media),
        token_media_id = case
            -- If there isn't any media, use the new media regardless of its status
            when token_media_id is null then (select id from insert_new_media)
            -- Otherwise, only replace reference to new media if it is active
            when @new_media_is_active then (select id from insert_new_media)
            -- If it isn't, keep the existing media
            else token_definitions.token_media_id
        end
    where (chain, contract_address, token_id) = (@chain, @contract_address, @token_id) and not deleted
)
-- Always return the new media that was inserted, even if its inactive so the pipeline can report metrics accurately
select sqlc.embed(token_medias) from insert_new_media token_medias;

-- name: InsertSpamContracts :exec
with insert_spam_contracts as (
    insert into alchemy_spam_contracts (id, chain, address, created_at, is_spam) (
        select unnest(@id::varchar[])
        , unnest(@chain::int[])
        , unnest(@address::varchar[])
        , unnest(@created_at::timestamptz[])
        , unnest(@is_spam::bool[])
    ) on conflict(chain, address) do update set created_at = excluded.created_at, is_spam = excluded.is_spam
    returning created_at
)
delete from alchemy_spam_contracts where created_at < (select created_at from insert_spam_contracts limit 1);

-- name: GetPushTokenByPushToken :one
select * from push_notification_tokens where push_token = @push_token and deleted = false;

-- name: CreatePushTokenForUser :one
insert into push_notification_tokens (id, user_id, push_token, created_at, deleted) values (@id, @user_id, @push_token, now(), false) returning *;

-- name: DeletePushTokensByIDs :exec
update push_notification_tokens set deleted = true where id = any(@ids::dbid[]) and deleted = false;

-- name: GetPushTokensByUserID :many
select * from push_notification_tokens where user_id = @user_id and deleted = false;

-- name: GetPushTokensByIDs :many
with keys as (
    select unnest (@ids::text[]) as id
         , generate_subscripts(@ids::text[], 1) as index
)
select t.* from keys k join push_notification_tokens t on t.id = k.id and t.deleted = false
    order by k.index;

-- name: CreatePushTickets :exec
insert into push_notification_tickets (id, push_token_id, ticket_id, created_at, check_after, num_check_attempts, status, deleted) values
  (
   unnest(@ids::text[]),
   unnest(@push_token_ids::text[]),
   unnest(@ticket_ids::text[]),
   now(),
   now() + interval '15 minutes',
   0,
   'pending',
   false
  );

-- name: UpdatePushTickets :exec
with updates as (
    select unnest(@ids::text[]) as id, unnest(@check_after::timestamptz[]) as check_after, unnest(@num_check_attempts::int[]) as num_check_attempts, unnest(@status::text[]) as status, unnest(@deleted::bool[]) as deleted
)
update push_notification_tickets t set check_after = updates.check_after, num_check_attempts = updates.num_check_attempts, status = updates.status, deleted = updates.deleted from updates where t.id = updates.id and t.deleted = false;

-- name: GetCheckablePushTickets :many
select * from push_notification_tickets where check_after <= now() and deleted = false limit sqlc.arg('limit');

-- name: GetSVGTokensWithContractsByIDs :many
SELECT
    tokens.*,
    contracts.*,
    (
        SELECT wallets.address
        FROM wallets
        WHERE wallets.id = ANY(tokens.owned_by_wallets) and wallets.deleted = false
        LIMIT 1
    ) AS wallet_address
FROM tokens
JOIN contracts ON contracts.id = tokens.contract_id
LEFT JOIN token_medias on token_medias.id = tokens.token_media_id
WHERE tokens.deleted = false
AND token_medias.active = true
AND token_medias.media->>'media_type' = 'svg'
AND tokens.id >= @start_id AND tokens.id < @end_id
ORDER BY tokens.id;

-- name: GetReprocessJobRangeByID :one
select * from reprocess_jobs where id = $1;

-- name: GetMediaByMediaIdIgnoringStatusBatch :batchone
select m.* from token_medias m where m.id = $1 and not deleted;

-- name: GetMediaByTokenIdentifiersIgnoringStatus :one
select tm.*
from token_definitions td
join token_medias tm on td.token_media_id = tm.id
where (td.chain, td.contract_address, td.token_id) = (@chain, @contract_address::address, @token_id::hextokenid)
    and not td.deleted
    and not tm.deleted;

-- name: UpsertSession :one
insert into sessions (id, user_id,
                      created_at, created_with_user_agent, created_with_platform, created_with_os,
                      last_refreshed, last_user_agent, last_platform, last_os, current_refresh_id, active_until, invalidated, last_updated, deleted)
    values (@id, @user_id, now(), @user_agent, @platform, @os, now(), @user_agent, @platform, @os, @current_refresh_id, @active_until, false, now(), false)
    on conflict (id) where deleted = false do update set
        last_refreshed = case when sessions.invalidated then sessions.last_refreshed else excluded.last_refreshed end,
        last_user_agent = case when sessions.invalidated then sessions.last_user_agent else excluded.last_user_agent end,
        last_platform = case when sessions.invalidated then sessions.last_platform else excluded.last_platform end,
        last_os = case when sessions.invalidated then sessions.last_os else excluded.last_os end,
        current_refresh_id = case when sessions.invalidated then sessions.current_refresh_id else excluded.current_refresh_id end,
        last_updated = case when sessions.invalidated then sessions.last_updated else excluded.last_updated end,
        active_until = case when sessions.invalidated then sessions.active_until else greatest(sessions.active_until, excluded.active_until) end
    returning *;

-- name: InvalidateSession :exec
update sessions set invalidated = true, active_until = least(active_until, now()), last_updated = now() where id = @id and deleted = false and invalidated = false;

-- name: UpdateTokenMetadataFieldsByTokenIdentifiers :one
update token_definitions
set name = @name,
    description = @description,
    last_updated = now()
where token_id = @token_id
    and contract_id = @contract_id
    and chain = @chain
    and deleted = false
returning *;

-- name: GetTopCollectionsForCommunity :many
with contract_tokens as (
	select t.id, t.owner_user_id
	from tokens t
	join contracts c on t.contract_id = c.id
	where not t.deleted
	  and not c.deleted
	  and t.displayable
	  and c.chain = $1
	  and c.address = $2
),
ranking as (
	select col.id, rank() over (order by count(col.id) desc, col.created_at desc) score
	from collections col
	join contract_tokens on col.owner_user_id = contract_tokens.owner_user_id and contract_tokens.id = any(col.nfts)
	join users on col.owner_user_id = users.id
	where not col.deleted and not col.hidden and not users.deleted
	group by col.id
)
select collections.id from collections join ranking using(id) where score <= 100 order by score asc;

-- name: GetVisibleCollectionsByIDsPaginateBatch :batchmany
select collections.*
from collections, unnest(@collection_ids::varchar[]) with ordinality as t(id, pos)
where collections.id = t.id and not deleted and not hidden and t.pos > @cur_after_pos::int and t.pos < @cur_before_pos::int
order by t.pos asc;

-- name: SetContractOverrideCreator :exec
update contracts set override_creator_user_id = @creator_user_id, last_updated = now() where id = @contract_id and deleted = false;

-- name: RemoveContractOverrideCreator :exec
update contracts set override_creator_user_id = null, last_updated = now() where id = @contract_id and deleted = false;

-- name: SetProfileImageToToken :exec
with new_image as (
    insert into profile_images (id, user_id, source_type, token_id, deleted, last_updated)
    values (@profile_id, @user_id, @token_source_type, @token_id, false, now())
    on conflict (user_id) do update set token_id = excluded.token_id
        , source_type = excluded.source_type
        , deleted = excluded.deleted
        , last_updated = excluded.last_updated
    returning id
)
update users set profile_image_id = new_image.id from new_image where users.id = @user_id and not deleted;

-- name: SetProfileImageToENS :one
with profile_images as (
    insert into profile_images (id, user_id, source_type, wallet_id, ens_domain, ens_avatar_uri, deleted, last_updated)
    values (@profile_id, @user_id, @ens_source_type, @wallet_id, @ens_domain, @ens_avatar_uri, false, now())
    on conflict (user_id) do update set wallet_id = excluded.wallet_id
        , ens_domain = excluded.ens_domain
        , ens_avatar_uri = excluded.ens_avatar_uri
        , source_type = excluded.source_type
        , deleted = excluded.deleted
        , last_updated = excluded.last_updated
    returning *
)
update users set profile_image_id = profile_images.id from profile_images where users.id = @user_id and not users.deleted returning sqlc.embed(profile_images);

-- name: RemoveProfileImage :exec
with remove_image as (
    update profile_images set deleted = true, last_updated = now() where user_id = $1 and not deleted
)
update users set profile_image_id = null where users.id = $1 and not users.deleted;

-- name: GetProfileImageByIdBatch :batchone
select * from profile_images pfp
where pfp.id = @id
	and not deleted
	and case
		when pfp.source_type = @ens_source_type
		then exists(select 1 from wallets w where w.id = pfp.wallet_id and not w.deleted)
		when pfp.source_type = @token_source_type
		then exists(select 1 from tokens t where t.id = pfp.token_id and not t.deleted)
		else
		0 = 1
	end;

-- name: GetPotentialENSProfileImageByUserId :one
select sqlc.embed(token_definitions), sqlc.embed(token_medias), sqlc.embed(wallets)
from token_definitions, tokens, users, token_medias, wallets, unnest(tokens.owned_by_wallets) tw(id)
where token_definitions.contract_address = @ens_address
    and token_definitions.chain = @chain
    and tokens.owner_user_id = @user_id
    and users.id = tokens.owner_user_id
    and tw.id = wallets.id
    and token_definitions.id = tokens.token_definition_id
    and token_definitions.token_media_id = token_medias.id
    and token_medias.active
    and nullif(token_medias.media->>'profile_image_url', '') is not null
    and not users.deleted
    and not token_medias.deleted
    and not wallets.deleted
    and not token_definitions.deleted
    and not tokens.deleted
order by tw.id = users.primary_wallet_id desc, tokens.id desc
limit 1;

-- name: GetCurrentTime :one
select now()::timestamptz;

-- name: GetUsersByWalletAddressesAndChains :many
WITH params AS (
    SELECT unnest(@wallet_addresses::varchar[]) as address, unnest(@chains::int[]) as chain
)
SELECT sqlc.embed(wallets), sqlc.embed(users)
FROM wallets 
JOIN users ON wallets.id = any(users.wallets)
JOIN params ON wallets.address = params.address AND wallets.chain = params.chain
WHERE not wallets.deleted AND not users.deleted and not users.universal;

-- name: GetUniqueTokenIdentifiersByTokenID :one
select token_definitions.token_id, token_definitions.contract_address, token_definitions.chain, tokens.quantity, array_agg(wallets.address)::varchar[] as owner_addresses 
from tokens
join token_definitions on tokens.token_definition_id = token_definitions.id
join wallets on wallets.id = any(tokens.owned_by_wallets)
where tokens.id = $1 and tokens.displayable and not tokens.deleted and not token_definitions.deleted and not wallets.deleted
group by (token_definitions.token_id, token_definitions.contract_address, token_definitions.chain, tokens.quantity) limit 1;

-- name: GetCreatedContractsByUserID :many
select sqlc.embed(c),
       w.id as wallet_id,
       false as is_override_creator
from users u, contracts c, wallets w
where u.id = @user_id
  and c.chain = any(@chains::int[])
  and w.id = any(u.wallets) and coalesce(nullif(c.owner_address, ''), nullif(c.creator_address, '')) = w.address 
  and w.l1_chain = c.l1_chain
  and u.deleted = false
  and c.deleted = false
  and w.deleted = false
  and c.override_creator_user_id is null
  and (not @new_contracts_only::bool or not exists(
    select 1 from tokens t
        where t.owner_user_id = @user_id
          and t.contract_id = c.id
          and t.is_creator_token
          and not t.deleted
        )
    )

union all

select sqlc.embed(c),
       null as wallet_id,
       true as is_override_creator
from contracts c
where c.override_creator_user_id = @user_id
  and c.chain = any(@chains::int[])
  and c.deleted = false
  and (not @new_contracts_only::bool or not exists(
    select 1 from tokens t
        where t.owner_user_id = @user_id
          and t.contract_id = c.id
          and t.is_creator_token
          and not t.deleted
        )
    );

-- name: RemoveWalletFromTokens :exec
update tokens t
    set owned_by_wallets = array_remove(owned_by_wallets, @wallet_id::varchar),
        last_updated = now()
    from users u
    where u.id = @user_id
      and t.owner_user_id = u.id
      and t.owned_by_wallets @> array[@wallet_id::varchar]
      and not u.wallets @> array[@wallet_id::varchar]
      and not u.deleted
      and not t.deleted;

-- name: RemoveStaleCreatorStatusFromTokens :exec
with created_contracts as (
    select * from contract_creators where creator_user_id = @user_id
)
update tokens
    set is_creator_token = false,
        last_updated = now()
    where owner_user_id = @user_id
      and is_creator_token = true
      and not exists(select 1 from created_contracts where created_contracts.contract_id = tokens.contract_id)
      and not deleted;

-- name: GetUsersBySocialIDs :many
select * from pii.user_view u where u.pii_socials->sqlc.arg('social_account_type')::varchar->>'id' = any(@social_ids::varchar[]) and not u.deleted and not u.universal;

-- name: InsertCommentMention :one
insert into mentions (id, user_id, community_id, comment_id, start, length) values (@id, sqlc.narg('user')::text, sqlc.narg('community')::text, @comment_id, @start, @length) returning *;

-- name: InsertPostMention :one
insert into mentions (id, user_id, community_id, post_id, start, length) values (@id, sqlc.narg('user')::text, sqlc.narg('community')::text, @post_id, @start, @length) returning *;

-- name: GetMentionsByCommentID :batchmany
select * from mentions where comment_id = @comment_id and not deleted;

-- name: GetMentionsByPostID :batchmany
select * from mentions where post_id = @post_id and not deleted;

-- name: GetMentionByID :one
select * from mentions where id = @id and not deleted;

-- name: GetUsersWithoutSocials :many
select u.id, w.address, u.pii_socials->>'Lens' is null, u.pii_socials->>'Farcaster' is null from pii.user_view u join wallets w on w.id = any(u.wallets) where u.deleted = false and w.chain = 0 and w.deleted = false and u.universal = false and (u.pii_socials->>'Lens' is null or u.pii_socials->>'Farcaster' is null) order by u.created_at desc;

-- name: GetMostActiveUsers :many
WITH ag AS (
    SELECT actor_id, COUNT(*) AS admire_given
    FROM admires
    WHERE created_at >= NOW() - INTERVAL '7 days' AND deleted = false
    GROUP BY actor_id
),
ar AS (
    SELECT p.actor_id, COUNT(*) AS admire_received
    FROM posts p
    JOIN admires a ON p.id = a.post_id
    WHERE a.created_at >= NOW() - INTERVAL '7 days' AND a.deleted = false
    GROUP BY p.actor_id
),
cm AS (
    SELECT actor_id, COUNT(id) AS comments_made
    FROM comments
    WHERE created_at >= NOW() - INTERVAL '7 days' AND deleted = false and removed = false
    GROUP BY actor_id
),
cr AS (
    SELECT p.actor_id, COUNT(c.id) AS comments_received
    FROM posts p
    JOIN comments c ON p.id = c.post_id
    WHERE p.created_at >= NOW() - INTERVAL '7 days' AND c.deleted = false and c.removed = false
    GROUP BY p.actor_id
),
scores AS (
    SELECT 
        ((COALESCE(ar.admire_received, 0) * @admire_received_weight::int) + 
        (COALESCE(ag.admire_given, 0) * @admire_given_weight::int) + 
        (COALESCE(cm.comments_made, 0) * @comments_made_weight::int) + 
        (COALESCE(cr.comments_received, 0) * @comments_received_weight::int)) AS score,
        COALESCE(nullif(ag.actor_id,''), nullif(ar.actor_id,''), nullif(cm.actor_id,''), nullif(cr.actor_id,'')) AS actor_id,
        COALESCE(ag.admire_given, 0) AS admires_given,
        COALESCE(ar.admire_received, 0) AS admires_received,
        COALESCE(cm.comments_made, 0) AS comments_made,
        COALESCE(cr.comments_received, 0) AS comments_received
    FROM ag
    FULL OUTER JOIN ar using(actor_id)
    FULL OUTER JOIN cm using(actor_id)
    FULL OUTER JOIN cr using(actor_id)
)
SELECT scores.*, users.traits
FROM scores
join users on scores.actor_id = users.id
WHERE users.deleted = false AND users.universal = false
AND scores.actor_id IS NOT NULL AND scores.score > 0
ORDER BY scores.score DESC
LIMIT $1;

-- name: UpdateTopActiveUsers :exec
UPDATE users
SET traits = CASE 
                WHEN id = ANY(@top_user_ids::dbid[]) THEN
                    COALESCE(traits, '{}'::jsonb) || '{"top_activity": true}'::jsonb
                ELSE 
                    traits - 'top_activity'
             END
WHERE id = ANY(@top_user_ids) OR traits ? 'top_activity';

-- name: GetOnboardingUserRecommendations :many
with sources as (
    select id from users where (traits->>'top_activity')::bool
    union all select recommended_user_id from top_recommended_users
    union all select user_id from user_internal_recommendations
), top_recs as (select sources.id from sources group by sources.id order by count(id) desc, random())
select users.* from users join top_recs using(id) where not users.deleted and not users.universal limit $1;

-- name: InsertMention :one
insert into mentions (id, comment_id, user_id, community_id, start, length) values ($1, $2, sqlc.narg('user')::text, sqlc.narg('community')::text, $3, $4) returning id;

-- name: InsertComment :one
INSERT INTO comments 
(ID, FEED_EVENT_ID, POST_ID, ACTOR_ID, REPLY_TO, TOP_LEVEL_COMMENT_ID, COMMENT) 
VALUES 
(sqlc.arg('id'), sqlc.narg('feed_event')::varchar, sqlc.narg('post')::varchar, sqlc.arg('actor_id'), sqlc.narg('reply')::varchar, 
    (CASE 
        WHEN sqlc.narg('reply')::varchar IS NOT NULL THEN
            (SELECT COALESCE(c.top_level_comment_id, sqlc.narg('reply')::varchar) 
             FROM comments c 
             WHERE c.id = sqlc.narg('reply')::varchar)
        ELSE NULL 
    END), 
sqlc.arg('comment')::varchar) 
RETURNING ID;

-- name: RemoveComment :exec
UPDATE comments SET REMOVED = TRUE, COMMENT = 'comment removed' WHERE ID = $1;

-- name: ReportPost :one
with offending_post as (select id from posts where posts.id = @post_id and not deleted)
insert into reported_posts (id, post_id, reporter_id, reason) (select @id, offending_post.id, sqlc.narg(reporter)::text, sqlc.narg(reason) from offending_post)
on conflict(post_id, reporter_id, reason) where not deleted do update set last_updated = now() returning id;

-- name: BlockUser :one
with user_to_block as (select id from users where users.id = @blocked_user_id and not deleted and not universal)
insert into user_blocklist (id, user_id, blocked_user_id, active) (select @id, @user_id, user_to_block.id, true from user_to_block)
on conflict(user_id, blocked_user_id) where not deleted do update set active = true, last_updated = now() returning id;

-- name: UnblockUser :exec
update user_blocklist set active = false, last_updated = now() where user_id = @user_id and blocked_user_id = @blocked_user_id and not deleted;

-- name: GetTopCommunitiesByPosts :many
with post_report as (
    select posts.id post_id, unnest(token_ids) token_id
    from posts
    where posts.created_at >= @window_end and not posts.deleted
)
select coalesce(token_community.id, contract_community.id) id
from post_report
    join tokens on post_report.token_id = tokens.id
    join token_definitions on tokens.token_definition_id = token_definitions.id
    left join token_community_memberships on token_definitions.id = token_community_memberships.token_definition_id
    left join communities token_community on token_community_memberships.community_id = token_community.id
    left join communities contract_community on token_definitions.contract_id = contract_community.contract_id
    left join admires on post_report.post_id = admires.post_id
    left join comments on comments.post_id = post_report.post_id
where not admires.deleted and not comments.deleted and not tokens.deleted
group by coalesce(token_community.id, contract_community.id)
order by count(distinct admires.id) + count(distinct comments.id) desc;

-- name: GetTopGalleriesByViews :many
with viewed_galleries as (
  select gallery_id id, count(id) views
  from events
  where action = 'ViewedGallery' and events.created_at > @window_end
  group by gallery_id
),
updated_galleries as (
    select galleries.id
    from events
    join galleries on events.collection_id = any(galleries.collections)
    where events.created_at > @window_end and action = 'TokensAddedToCollection' and not galleries.deleted
    group by galleries.id
)
select viewed_galleries.id
from viewed_galleries
join updated_galleries using(id)
order by viewed_galleries.views desc;

-- name: GetCommunitiesByTokenDefinitionID :batchmany
select communities.* from communities
    join token_definitions on token_definitions.contract_id = communities.contract_id
    where community_type = 0
        and token_definitions.id = @token_definition_id
        and not communities.deleted
        and not token_definitions.deleted

union all

select communities.* from communities
    join token_community_memberships on token_community_memberships.community_id = communities.id
    where community_type != 0
        and token_community_memberships.token_definition_id = @token_definition_id
        and not communities.deleted
        and not token_community_memberships.deleted;


-- name: GetActiveWallets :many
select w.* from users u join wallets w on w.id = any(u.wallets) where not u.deleted and not w.deleted and not u.universal;

-- name: GetGalleriesDisplayingCommunityIDPaginateBatch :batchmany
select sqlc.embed(g),
       cg.token_ids as community_token_ids,
       cg.token_medias as community_medias,
       cg.token_media_last_updated::timestamptz[] as community_media_last_updated,
       cg2.token_ids as all_token_ids,
       cg2.token_medias as all_medias,
       cg2.token_media_last_updated::timestamptz[] as all_media_last_updated,
       (-cg.gallery_relevance)::float8 as relevance from community_galleries cg
    join galleries g on cg.gallery_id = g.id and not g.deleted and not g.hidden
    join community_galleries cg2 on cg2.gallery_id = cg.gallery_id and cg2.community_id is null
where cg.community_id = @community_id
    and (cg.user_id != @relative_to_user_id, cg.community_id, -cg.gallery_relevance, cg.gallery_id) < (@cur_before_is_not_relative_user::bool, @community_id, @cur_before_relevance::float8, @cur_before_id::dbid)
    and (cg.user_id != @relative_to_user_id, cg.community_id, -cg.gallery_relevance, cg.gallery_id) > (@cur_after_is_not_relative_user::bool, @community_id, @cur_after_relevance::float8, @cur_after_id::dbid)
order by case when @paging_forward::bool then (cg.user_id != @relative_to_user_id, cg.community_id, -cg.gallery_relevance, cg.gallery_id) end asc,
         case when not @paging_forward::bool then (cg.user_id != @relative_to_user_id, cg.community_id, -cg.gallery_relevance, cg.gallery_id) end desc
limit sqlc.arg('limit');

-- name: CountGalleriesDisplayingCommunityIDBatch :batchone
select count(*) from community_galleries cg
    join galleries g on cg.gallery_id = g.id and not g.deleted and not g.hidden
where cg.community_id = @community_id;

-- name: SetPersonaByUserID :exec
update users set persona = @persona where id = @user_id and not deleted;

-- name: GetUserByPrivyDID :one
select u.* from
    privy_users p
        join users u on p.user_id = u.id and not u.deleted
where
    p.privy_did = @privy_did
    and not p.deleted;

-- name: SetPrivyDIDForUser :exec
insert into privy_users (id, user_id, privy_did)
    values (@id, @user_id, @privy_did)
    on conflict (user_id) where not deleted do update set privy_did = excluded.privy_did, last_updated = now();

-- name: SaveHighlightMintClaim :one
insert into highlight_mint_claims(
    id
    , recipient_user_id
    , recipient_l1_chain
    , recipient_address
    , recipient_wallet_id
    , highlight_collection_id
    , highlight_claim_id
    , collection_address
    , collection_chain
    , status
    , error_message
)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) returning id;

-- name: GetHighlightMintClaimByID :one
select * from highlight_mint_claims where id = $1 and not deleted;

-- name: HasMintedClaimsByUserID :one
with minted  as ( select 1 from highlight_mint_claims m where m.recipient_user_id = $1 and m.highlight_collection_id = $2 and m.status = any(@minted_statuses::varchar[]) and not m.deleted ),
     pending as ( select 1 from highlight_mint_claims p where p.recipient_user_id = $1 and p.highlight_collection_id = $2 and p.status = any(@pending_statuses::varchar[]) and not p.deleted )
select exists(select 1 from minted) has_minted, exists(select 1 from pending) has_pending;

-- name: HasMintedClaimsByWalletAddress :one
with minted  as ( select 1 from highlight_mint_claims m where m.recipient_l1_chain = $1 and m.recipient_address = $2 and m.highlight_collection_id = $3 and m.status = any(@minted_statuses::varchar[]) and not m.deleted ),
     pending as ( select 1 from highlight_mint_claims p where p.recipient_l1_chain = $1 and p.recipient_address = $2 and p.highlight_collection_id = $3 and p.status = any(@pending_statuses::varchar[]) and not p.deleted )
select exists(select 1 from minted) has_minted, exists(select 1 from pending) has_pending;

-- name: UpdateHighlightMintClaimStatus :one
update highlight_mint_claims set last_updated = now(), status = $1, error_message = $2 where id = @id returning *;

-- name: UpdateHighlightMintClaimStatusTxSucceeded :one
update highlight_mint_claims set last_updated = now(), status = $1, minted_token_id = $2, minted_token_metadata = $3 where id = @id returning *;

-- name: UpdateHighlightMintClaimStatusMediaProcessing :one
update highlight_mint_claims set last_updated = now(), status = $1, internal_token_id = $2 where id = @id returning *;
