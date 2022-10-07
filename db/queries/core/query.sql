-- name: GetUserById :one
SELECT * FROM users WHERE id = $1 AND deleted = false;

-- name: GetUserByIdBatch :batchone
SELECT * FROM users WHERE id = $1 AND deleted = false;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username_idempotent = lower(sqlc.arg(username)) AND deleted = false;

-- name: GetUserByUsernameBatch :batchone
SELECT * FROM users WHERE username_idempotent = lower($1) AND deleted = false;

-- name: GetUsersWithTrait :many
SELECT * FROM users WHERE (traits->$1::string) IS NOT NULL AND deleted = false;

-- name: GetUsersWithTraitBatch :batchmany
SELECT * FROM users WHERE (traits->$1::string) IS NOT NULL AND deleted = false;

-- name: GetGalleryById :one
SELECT * FROM galleries WHERE id = $1 AND deleted = false;

-- name: GetGalleryByIdBatch :batchone
SELECT * FROM galleries WHERE id = $1 AND deleted = false;

-- name: GetGalleryByCollectionId :one
SELECT g.* FROM galleries g, collections c WHERE c.id = $1 AND c.deleted = false AND $1 = ANY(g.collections) AND g.deleted = false;

-- name: GetGalleryByCollectionIdBatch :batchone
SELECT g.* FROM galleries g, collections c WHERE c.id = $1 AND c.deleted = false AND $1 = ANY(g.collections) AND g.deleted = false;

-- name: GetGalleriesByUserId :many
SELECT * FROM galleries WHERE owner_user_id = $1 AND deleted = false;

-- name: GetGalleriesByUserIdBatch :batchmany
SELECT * FROM galleries WHERE owner_user_id = $1 AND deleted = false;

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
SELECT * FROM tokens WHERE id = $1 AND deleted = false;

-- name: GetTokenByIdBatch :batchone
SELECT * FROM tokens WHERE id = $1 AND deleted = false;

-- name: GetTokensByCollectionId :many
SELECT t.* FROM users u, collections c, unnest(c.nfts)
    WITH ORDINALITY AS x(nft_id, nft_ord)
    INNER JOIN tokens t ON t.id = x.nft_id
    WHERE u.id = t.owner_user_id AND t.owned_by_wallets && u.wallets
    AND c.id = $1 AND u.deleted = false AND c.deleted = false AND t.deleted = false ORDER BY x.nft_ord;

-- name: GetTokensByCollectionIdBatch :batchmany
SELECT t.* FROM users u, collections c, unnest(c.nfts)
    WITH ORDINALITY AS x(nft_id, nft_ord)
    INNER JOIN tokens t ON t.id = x.nft_id
    WHERE u.id = t.owner_user_id AND t.owned_by_wallets && u.wallets
    AND c.id = $1 AND u.deleted = false AND c.deleted = false AND t.deleted = false ORDER BY x.nft_ord;

-- name: GetNewTokensByFeedEventIdBatch :batchmany
WITH new_tokens AS (
    SELECT added.id, row_number() OVER () added_order
    FROM (SELECT jsonb_array_elements_text(data -> 'collection_new_token_ids') id FROM feed_events f WHERE f.id = $1 AND f.deleted = false) added
)
SELECT t.* FROM new_tokens a JOIN tokens t ON a.id = t.id AND t.deleted = false ORDER BY a.added_order;

-- name: GetMembershipByMembershipId :one
SELECT * FROM membership WHERE id = $1 AND deleted = false;

-- name: GetMembershipByMembershipIdBatch :batchone
SELECT * FROM membership WHERE id = $1 AND deleted = false;

-- name: GetWalletByID :one
SELECT * FROM wallets WHERE id = $1 AND deleted = false;

-- name: GetWalletByIDBatch :batchone
SELECT * FROM wallets WHERE id = $1 AND deleted = false;

-- name: GetWalletByChainAddress :one
SELECT wallets.* FROM wallets WHERE address = $1 AND chain = $2 AND deleted = false;

-- name: GetWalletByChainAddressBatch :batchone
SELECT wallets.* FROM wallets WHERE address = $1 AND chain = $2 AND deleted = false;

-- name: GetWalletsByUserID :many
SELECT w.* FROM users u, unnest(u.wallets) WITH ORDINALITY AS a(wallet_id, wallet_ord)INNER JOIN wallets w on w.id = a.wallet_id WHERE u.id = $1 AND u.deleted = false AND w.deleted = false ORDER BY a.wallet_ord;

-- name: GetWalletsByUserIDBatch :batchmany
SELECT w.* FROM users u, unnest(u.wallets) WITH ORDINALITY AS a(wallet_id, wallet_ord)INNER JOIN wallets w on w.id = a.wallet_id WHERE u.id = $1 AND u.deleted = false AND w.deleted = false ORDER BY a.wallet_ord;

-- name: GetContractByID :one
select * FROM contracts WHERE id = $1 AND deleted = false;

-- name: GetContractsByIDs :many
SELECT * from contracts WHERE id = ANY(@contract_ids) AND deleted = false;

-- name: GetContractByChainAddress :one
select * FROM contracts WHERE address = $1 AND chain = $2 AND deleted = false;

-- name: GetContractByChainAddressBatch :batchone
select * FROM contracts WHERE address = $1 AND chain = $2 AND deleted = false;

-- name: GetContractsByUserID :many
SELECT DISTINCT ON (contracts.id) contracts.* FROM contracts, tokens
    WHERE tokens.owner_user_id = $1 AND tokens.contract = contracts.id
    AND tokens.deleted = false AND contracts.deleted = false;

-- name: GetContractsByUserIDBatch :batchmany
SELECT DISTINCT ON (contracts.id) contracts.* FROM contracts, tokens
    WHERE tokens.owner_user_id = $1 AND tokens.contract = contracts.id
    AND tokens.deleted = false AND contracts.deleted = false;

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

-- name: GetTokensByWalletIds :many
SELECT * FROM tokens WHERE owned_by_wallets && $1 AND deleted = false
    ORDER BY tokens.created_at DESC, tokens.name DESC, tokens.id DESC;

-- name: GetTokensByWalletIdsBatch :batchmany
SELECT * FROM tokens WHERE owned_by_wallets && $1 AND deleted = false
    ORDER BY tokens.created_at DESC, tokens.name DESC, tokens.id DESC;

-- name: GetTokensByUserId :many
SELECT tokens.* FROM tokens, users
    WHERE tokens.owner_user_id = $1 AND users.id = $1
      AND tokens.owned_by_wallets && users.wallets
      AND tokens.deleted = false AND users.deleted = false
    ORDER BY tokens.created_at DESC, tokens.name DESC, tokens.id DESC;

-- name: GetTokensByUserIdBatch :batchmany
SELECT tokens.* FROM tokens, users
    WHERE tokens.owner_user_id = $1 AND users.id = $1
      AND tokens.owned_by_wallets && users.wallets
      AND tokens.deleted = false AND users.deleted = false
    ORDER BY tokens.created_at DESC, tokens.name DESC, tokens.id DESC;

-- name: GetTokensByUserIdAndChainBatch :batchmany
SELECT tokens.* FROM tokens, users
WHERE tokens.owner_user_id = $1 AND users.id = $1
  AND tokens.owned_by_wallets && users.wallets
  AND tokens.deleted = false AND users.deleted = false
  AND tokens.chain = $2
ORDER BY tokens.created_at DESC, tokens.name DESC, tokens.id DESC;

-- name: CreateUserEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, user_id, subject_id, data) VALUES ($1, $2, $3, $4, $5, $5, $6) RETURNING *;

-- name: CreateTokenEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, token_id, subject_id, data) VALUES ($1, $2, $3, $4, $5, $5, $6) RETURNING *;

-- name: CreateCollectionEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, collection_id, subject_id, data) VALUES ($1, $2, $3, $4, $5, $5, $6) RETURNING *;

-- name: GetEvent :one
SELECT * FROM events WHERE id = $1 AND deleted = false;

-- name: GetEventsInWindow :many
WITH RECURSIVE activity AS (
    SELECT * FROM events WHERE events.id = $1 AND deleted = false
    UNION
    SELECT e.* FROM events e, activity a
    WHERE e.actor_id = a.actor_id
        AND e.action = a.action
        AND e.created_at < a.created_at
        AND e.created_at >= a.created_at - make_interval(secs => $2)
        AND e.deleted = false
)
SELECT * FROM events WHERE id = ANY(SELECT id FROM activity) ORDER BY created_at DESC;

-- name: IsWindowActive :one
SELECT EXISTS(
    SELECT 1 FROM events
    WHERE actor_id = $1 AND action = $2 AND deleted = false
    AND created_at > @window_start AND created_at <= @window_end
    LIMIT 1
);

-- name: IsWindowActiveWithSubject :one
SELECT EXISTS(
    SELECT 1 FROM events
    WHERE actor_id = $1 AND action = $2 AND subject_id = $3 AND deleted = false
    AND created_at > @window_start AND created_at <= @window_end
    LIMIT 1
);

-- name: GetGlobalFeedViewBatch :batchmany
WITH cursors AS (
    SELECT
    (SELECT CASE WHEN @cur_before::varchar = '' THEN now() ELSE (SELECT event_time FROM feed_events f WHERE f.id = @cur_before::varchar AND deleted = false) END) AS cur_before,
    (SELECT CASE WHEN @cur_after::varchar = '' THEN make_date(1970, 1, 1) ELSE (SELECT event_time FROM feed_events f WHERE f.id = @cur_after::varchar AND deleted = false) END) AS cur_after
), edges AS (
    SELECT id FROM feed_events
    WHERE event_time > (SELECT cur_after FROM cursors)
    AND event_time < (SELECT cur_before FROM cursors)
    AND deleted = false
), offsets AS (
    SELECT
        CASE WHEN NOT @from_first::bool AND count(id) - $1::int > 0
        THEN count(id) - $1::int
        ELSE 0 END pos
    FROM edges
)
SELECT * FROM feed_events
    WHERE id = ANY(SELECT id FROM edges)
    ORDER BY event_time ASC
    LIMIT $1 OFFSET (SELECT pos FROM offsets);

-- name: GlobalFeedHasMoreEvents :one
SELECT
    CASE WHEN @from_first::bool
    THEN EXISTS(
        SELECT 1
        FROM feed_events
        WHERE event_time > (SELECT event_time FROM feed_events f WHERE f.id = $1)
        AND deleted = false
        LIMIT 1
    )
    ELSE EXISTS(
        SELECT 1
        FROM feed_events
        WHERE event_time < (SELECT event_time FROM feed_events f WHERE f.id = $1)
        AND deleted = false
        LIMIT 1)
    END::bool;

-- name: GetUserFeedViewBatch :batchmany
WITH cursors AS (
    SELECT
    (SELECT CASE WHEN @cur_before::varchar = '' THEN now() ELSE (SELECT event_time FROM feed_events f WHERE f.id = @cur_before::varchar AND deleted = false) END) AS cur_before,
    (SELECT CASE WHEN @cur_after::varchar = '' THEN make_date(1970, 1, 1) ELSE (SELECT event_time FROM feed_events f WHERE f.id = @cur_after::varchar AND deleted = false) END) AS cur_after
), edges AS (
    SELECT fe.id FROM feed_events fe
    INNER JOIN follows fl ON fe.owner_id = fl.followee AND fl.follower = $1
    WHERE event_time > (SELECT cur_after FROM cursors)
    AND event_time < (SELECT cur_before FROM cursors)
    AND fe.deleted = false and fl.deleted = false
), offsets AS (
    SELECT
        CASE WHEN NOT @from_first::bool AND count(id) - $2::int > 0
        THEN count(id) - $2::int
        ELSE 0 END pos
    FROM edges
)
SELECT * FROM feed_events WHERE id = ANY(SELECT id FROM edges)
    ORDER BY event_time ASC
    LIMIT $2 OFFSET (SELECT pos FROM offsets);

-- name: UserFeedHasMoreEvents :one
SELECT
    CASE WHEN @from_first::bool
    THEN EXISTS(
        SELECT 1
        FROM feed_events fe
        INNER JOIN follows fl ON fe.owner_id = fl.followee AND fl.follower = $1
        WHERE event_time > (SELECT event_time FROM feed_events f WHERE f.id = $2)
        AND fe.deleted = false AND fl.deleted = false
        LIMIT 1)
    ELSE EXISTS(
        SELECT 1
        FROM feed_events fe
        INNER JOIN follows fl ON fe.owner_id = fl.followee AND fl.follower = $1
        WHERE event_time < (SELECT event_time FROM feed_events f WHERE f.id = $2)
        AND fe.deleted = false AND fl.deleted = false
        LIMIT 1
    )
    END::bool;

-- name: GetEventByIdBatch :batchone
SELECT * FROM feed_events WHERE id = $1 AND deleted = false;

-- name: CreateFeedEvent :one
INSERT INTO feed_events (id, owner_id, action, data, event_time, event_ids) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetLastFeedEvent :one
SELECT * FROM feed_events
    WHERE owner_id = $1 AND action = $2 AND event_time < $3 AND deleted = false
    ORDER BY event_time DESC
    LIMIT 1;

-- name: GetLastFeedEventForToken :one
SELECT * FROM feed_events
    WHERE owner_id = $1 and action = $2 AND data ->> 'token_id' = @token_id::varchar AND event_time < $3 AND deleted = false
    ORDER BY event_time DESC
    LIMIT 1;

-- name: GetLastFeedEventForCollection :one
SELECT * FROM feed_events
    WHERE owner_id = $1 and action = $2 AND data ->> 'collection_id' = @collection_id::varchar AND event_time < $3 AND deleted = false
    ORDER BY event_time DESC
    LIMIT 1;

-- name: IsFeedUserActionBlocked :one
SELECT EXISTS(SELECT 1 FROM feed_blocklist WHERE user_id = $1 AND action = $2 AND deleted = false);

-- name: GetAdmireByAdmireID :one
SELECT * FROM admires WHERE id = $1 AND deleted = false;

-- name: GetAdmiresByAdmireIDs :many
SELECT * from admires WHERE id = ANY(@admire_ids) AND deleted = false;

-- name: GetAdmireByAdmireIDBatch :batchone
SELECT * FROM admires WHERE id = $1 AND deleted = false;

-- name: GetAdmiresByActorID :many
SELECT * FROM admires WHERE actor_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: GetAdmiresByActorIDBatch :batchmany
SELECT * FROM admires WHERE actor_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: PaginateAdmiresByFeedEventIDForward :many
SELECT * FROM admires WHERE feed_event_id = $1 AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id) AND (created_at, id) > (@cur_after_time, @cur_after_id)
    ORDER BY (created_at, id) ASC LIMIT $2;

-- name: PaginateAdmiresByFeedEventIDBackward :many
SELECT * FROM admires WHERE feed_event_id = $1 AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id) AND (created_at, id) > (@cur_after_time, @cur_after_id)
    ORDER BY (created_at, id) DESC LIMIT $2;

-- name: CountAdmiresByFeedEventID :one
SELECT count(*) FROM admires WHERE feed_event_id = $1 AND deleted = false;

-- name: GetAdmiresByFeedEventID :many
SELECT * FROM admires WHERE feed_event_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: GetAdmiresByFeedEventIDBatch :batchmany
SELECT * FROM admires WHERE feed_event_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: GetCommentByCommentID :one
SELECT * FROM comments WHERE id = $1 AND deleted = false;

-- name: GetCommentsByCommentIDs :many
SELECT * from comments WHERE id = ANY(@comment_ids) AND deleted = false;

-- name: GetCommentByCommentIDBatch :batchone
SELECT * FROM comments WHERE id = $1 AND deleted = false;

-- name: PaginateCommentsByFeedEventIDForward :many
SELECT * FROM comments WHERE feed_event_id = $1 AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id)
    AND (created_at, id) > (@cur_after_time, @cur_after_id)
    ORDER BY (created_at, id) ASC LIMIT $2;

-- name: PaginateCommentsByFeedEventIDBackward :many
SELECT * FROM comments WHERE feed_event_id = $1 AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id)
    AND (created_at, id) > (@cur_after_time, @cur_after_id)
    ORDER BY (created_at, id) DESC LIMIT $2;

-- name: CountCommentsByFeedEventID :one
SELECT count(*) FROM comments WHERE feed_event_id = $1 AND deleted = false;

-- name: GetCommentsByActorID :many
SELECT * FROM comments WHERE actor_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: GetCommentsByActorIDBatch :batchmany
SELECT * FROM comments WHERE actor_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: GetCommentsByFeedEventID :many
SELECT * FROM comments WHERE feed_event_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: GetCommentsByFeedEventIDBatch :batchmany
SELECT * FROM comments WHERE feed_event_id = $1 AND deleted = false ORDER BY created_at DESC;
