-- name: GetUserById :one
SELECT * FROM users WHERE id = $1 AND deleted = false;

-- name: GetUserByIdBatch :batchone
SELECT * FROM users WHERE id = $1 AND deleted = false;

-- name: GetUsersByIDs :many
SELECT * FROM users WHERE id = ANY(@user_ids) AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id)
    AND (created_at, id) > (@cur_after_time, @cur_after_id)
    ORDER BY CASE WHEN @paging_forward::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT @paging_forward::bool THEN (created_at, id) END DESC
    LIMIT $1;

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

-- name: GetContractsDisplayedByUserIDBatch :batchmany
SELECT DISTINCT ON (contracts.id) contracts.* FROM contracts, tokens
JOIN collections c ON tokens.id = ANY(c.NFTS)
WHERE tokens.owner_user_id = $1 AND tokens.contract = contracts.id AND c.owner_user_id = tokens.owner_user_id
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

-- name: GetTokensByContractId :many
SELECT * FROM tokens WHERE contract = $1 AND deleted = false
    ORDER BY tokens.created_at DESC, tokens.name DESC, tokens.id DESC;

-- name: GetTokensByContractIdBatch :batchmany
SELECT * FROM tokens WHERE contract = $1 AND deleted = false
    ORDER BY tokens.created_at DESC, tokens.name DESC, tokens.id DESC;

-- name: GetTokensByContractIdPaginate :many
SELECT t.* FROM tokens t
    JOIN users u ON u.id = t.owner_user_id
    WHERE t.contract = $1 AND t.deleted = false
    AND (u.universal,t.created_at,t.id) < (@cur_before_universal, @cur_before_time::timestamptz, @cur_before_id)
    AND (u.universal,t.created_at,t.id) > (@cur_after_universal, @cur_after_time::timestamptz, @cur_after_id)
    ORDER BY CASE WHEN @paging_forward::bool THEN (u.universal,t.created_at,t.id) END ASC,
             CASE WHEN NOT @paging_forward::bool THEN (u.universal,t.created_at,t.id) END DESC
    LIMIT $2;

-- name: GetTokensByContractIdBatchPaginate :batchmany
SELECT t.* FROM tokens t
    JOIN users u ON u.id = t.owner_user_id
    WHERE t.contract = $1 AND t.deleted = false
    AND (u.universal,t.created_at,t.id) < (@cur_before_universal, @cur_before_time::timestamptz, @cur_before_id)
    AND (u.universal,t.created_at,t.id) > (@cur_after_universal, @cur_after_time::timestamptz, @cur_after_id)
    ORDER BY CASE WHEN @paging_forward::bool THEN (u.universal,t.created_at,t.id) END ASC,
             CASE WHEN NOT @paging_forward::bool THEN (u.universal,t.created_at,t.id) END DESC
    LIMIT $2;

-- name: CountTokensByContractId :one
SELECT count(*) FROM tokens WHERE contract = $1 AND deleted = false;

-- name: GetOwnersByContractIdBatchPaginate :batchmany
SELECT DISTINCT ON (result.id) result.* FROM (SELECT users.* FROM users, tokens
    WHERE tokens.contract = $1 AND tokens.owner_user_id = users.id
    AND tokens.deleted = false AND users.deleted = false
    AND (users.universal,users.created_at,users.id) < (@cur_before_universal, @cur_before_time::timestamptz, @cur_before_id)
    AND (users.universal,users.created_at,users.id) > (@cur_after_universal, @cur_after_time::timestamptz, @cur_after_id)
    ORDER BY CASE WHEN @paging_forward::bool THEN (users.universal,users.created_at,users.id) END ASC,
        CASE WHEN NOT @paging_forward::bool THEN (users.universal,users.created_at,users.id) END DESC) AS result LIMIT $2;

-- name: CountOwnersByContractId :one
SELECT count(DISTINCT users.id) FROM users, tokens
    WHERE tokens.contract = $1 AND tokens.owner_user_id = users.id
    AND tokens.deleted = false AND users.deleted = false;

-- name: CountGalleryOwnersByContractId :one
SELECT count(DISTINCT users.id) FROM users, tokens
    WHERE tokens.contract = $1 AND tokens.owner_user_id = users.id
    AND tokens.deleted = false AND users.deleted = false AND users.universal = false;

-- name: GetTokenOwnerByID :one
SELECT u.* FROM tokens t
    JOIN users u ON u.id = t.owner_user_id
    WHERE t.id = $1 AND t.deleted = false AND u.deleted = false;

-- name: GetTokenOwnerByIDBatch :batchone
SELECT u.* FROM tokens t
    JOIN users u ON u.id = t.owner_user_id
    WHERE t.id = $1 AND t.deleted = false AND u.deleted = false;

-- name: GetPreviewURLsByContractIdAndUserId :many
SELECT MEDIA->>'thumbnail_url'::varchar as thumbnail_url FROM tokens WHERE CONTRACT = $1 AND DELETED = false AND OWNER_USER_ID = $2 AND LENGTH(MEDIA->>'thumbnail_url'::varchar) > 0 ORDER BY ID LIMIT 3;

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

-- name: GetTokensByUserIdAndContractID :many
SELECT tokens.* FROM tokens, users
    WHERE tokens.owner_user_id = $1 AND users.id = $1
      AND tokens.owned_by_wallets && users.wallets
      AND tokens.contract = $2
      AND tokens.deleted = false AND users.deleted = false
    ORDER BY tokens.created_at DESC, tokens.name DESC, tokens.id DESC;

-- name: GetTokensByUserIdAndContractIDBatch :batchmany
SELECT tokens.* FROM tokens, users
    WHERE tokens.owner_user_id = $1 AND users.id = $1
      AND tokens.owned_by_wallets && users.wallets
      AND tokens.contract = $2
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

-- name: CreateGalleryEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, gallery_id, subject_id, data) VALUES ($1, $2, $3, $4, $5, $5, $6) RETURNING *;

-- name: CreateAdmireEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, admire_id, feed_event_id, subject_id, data) VALUES ($1, $2, $3, $4, $5, $6, $5, $7) RETURNING *;

-- name: CreateFeedEventEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, feed_event_id, subject_id, data) VALUES ($1, $2, $3, $4, $5, $5, $6) RETURNING *;

-- name: CreateCommentEvent :one
INSERT INTO events (id, actor_id, action, resource_type_id, comment_id, feed_event_id, subject_id, data) VALUES ($1, $2, $3, $4, $5, $6, $5, $7) RETURNING *;

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

-- name: PaginateGlobalFeed :batchmany
SELECT * FROM feed_events WHERE deleted = false
    AND (event_time, id) < (@cur_before_time, @cur_before_id)
    AND (event_time, id) > (@cur_after_time, @cur_after_id)
    ORDER BY CASE WHEN @paging_forward::bool THEN (event_time, id) END ASC,
            CASE WHEN NOT @paging_forward::bool THEN (event_time, id) END DESC
    LIMIT $1;

-- name: PaginatePersonalFeedByUserID :batchmany
SELECT fe.* FROM feed_events fe, follows fl WHERE fe.deleted = false AND fl.deleted = false
    AND fe.owner_id = fl.followee AND fl.follower = $1
    AND (fe.event_time, fe.id) < (@cur_before_time, @cur_before_id)
    AND (fe.event_time, fe.id) > (@cur_after_time, @cur_after_id)
    ORDER BY CASE WHEN @paging_forward::bool THEN (fe.event_time, fe.id) END ASC,
            CASE WHEN NOT @paging_forward::bool THEN (fe.event_time, fe.id) END DESC
    LIMIT $2;

-- name: PaginateUserFeedByUserID :batchmany
SELECT * FROM feed_events WHERE owner_id = $1 AND deleted = false
    AND (event_time, id) < (@cur_before_time, @cur_before_id)
    AND (event_time, id) > (@cur_after_time, @cur_after_id)
    ORDER BY CASE WHEN @paging_forward::bool THEN (event_time, id) END ASC,
            CASE WHEN NOT @paging_forward::bool THEN (event_time, id) END DESC
    LIMIT $2;

-- name: GetEventByIdBatch :batchone
SELECT * FROM feed_events WHERE id = $1 AND deleted = false;

-- name: CreateFeedEvent :one
INSERT INTO feed_events (id, owner_id, action, data, event_time, event_ids) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GeetFeedEventByID :one
SELECT * FROM feed_events WHERE id = $1 AND deleted = false;

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

-- name: PaginateAdmiresByFeedEventIDBatch :batchmany
SELECT * FROM admires WHERE feed_event_id = $1 AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id) AND (created_at, id) > (@cur_after_time, @cur_after_id)
    ORDER BY CASE WHEN @paging_forward::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT @paging_forward::bool THEN (created_at, id) END DESC
    LIMIT $2;

-- name: CountAdmiresByFeedEventIDBatch :batchone
SELECT count(*) FROM admires WHERE feed_event_id = $1 AND deleted = false;

-- name: GetCommentByCommentID :one
SELECT * FROM comments WHERE id = $1 AND deleted = false;

-- name: GetCommentsByCommentIDs :many
SELECT * from comments WHERE id = ANY(@comment_ids) AND deleted = false;

-- name: GetCommentByCommentIDBatch :batchone
SELECT * FROM comments WHERE id = $1 AND deleted = false;

-- name: PaginateCommentsByFeedEventIDBatch :batchmany
SELECT * FROM comments WHERE feed_event_id = $1 AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id)
    AND (created_at, id) > (@cur_after_time, @cur_after_id)
    ORDER BY CASE WHEN @paging_forward::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT @paging_forward::bool THEN (created_at, id) END DESC
    LIMIT $2;

-- name: CountCommentsByFeedEventIDBatch :batchone
SELECT count(*) FROM comments WHERE feed_event_id = $1 AND deleted = false;

-- name: GetCommentsByActorID :many
SELECT * FROM comments WHERE actor_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: GetCommentsByActorIDBatch :batchmany
SELECT * FROM comments WHERE actor_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: GetCommentsByFeedEventID :many
SELECT * FROM comments JOIN feed_events f on f.deleted = false AND f.id = $1 WHERE feed_event_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: GetCommentsByFeedEventIDBatch :batchmany
SELECT * FROM comments JOIN feed_events f on f.deleted = false AND f.id = $1 WHERE feed_event_id = $1 AND deleted = false ORDER BY created_at DESC;

-- name: GetUserNotifications :many
SELECT * FROM notifications WHERE owner_id = $1 AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id)
    AND (created_at, id) > (@cur_after_time, @cur_after_id)
    ORDER BY CASE WHEN @paging_forward::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT @paging_forward::bool THEN (created_at, id) END DESC
    LIMIT $2;

-- name: GetUserNotificationsBatch :batchmany
SELECT * FROM notifications WHERE owner_id = $1 AND deleted = false
    AND (created_at, id) < (@cur_before_time, @cur_before_id)
    AND (created_at, id) > (@cur_after_time, @cur_after_id)
    ORDER BY CASE WHEN @paging_forward::bool THEN (created_at, id) END ASC,
             CASE WHEN NOT @paging_forward::bool THEN (created_at, id) END DESC
    LIMIT $2;

-- name: CountUserNotifications :one
SELECT count(*) FROM notifications WHERE owner_id = $1 AND deleted = false;

-- name: GetNotificationByID :one
SELECT * FROM notifications WHERE id = $1 AND deleted = false;

-- name: GetNotificationByIDBatch :batchone
SELECT * FROM notifications WHERE id = $1 AND deleted = false;

-- name: GetMostRecentNotificationByOwnerIDForAction :one
SELECT * FROM notifications
    WHERE owner_id = $1 AND action = $2 AND deleted = false
    ORDER BY created_at DESC
    LIMIT 1;

-- name: GetNotificationsByOwnerIDForActionAfter :many
SELECT * FROM notifications
    WHERE owner_id = $1 AND action = $2 AND deleted = false AND created_at > @created_after
    ORDER BY created_at DESC;

-- name: CreateAdmireNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids, feed_event_id) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: CreateCommentNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids, feed_event_id, comment_id) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: CreateFollowNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: CreateViewGalleryNotification :one
INSERT INTO notifications (id, owner_id, action, data, event_ids, gallery_id) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: UpdateNotification :exec
UPDATE notifications SET data = $2, event_ids = event_ids || $3, amount = amount + $4, last_updated = now() WHERE id = $1;

-- name: UpdateNotificationSettingsByID :exec
UPDATE users SET notification_settings = $2 WHERE id = $1;

-- name: ClearNotificationsForUser :many
UPDATE notifications SET seen = true WHERE owner_id = $1 AND seen = false RETURNING *;

-- name: PaginateInteractionsByFeedEventIDBatch :batchmany
SELECT interactions.created_At, interactions.id, interactions.tag FROM (
    SELECT t.created_at, t.id, @admire_tag::int as tag FROM admires t WHERE @admire_tag != 0 AND t.feed_event_id = $1 AND t.deleted = false
        AND (t.created_at, t.id) < (@cur_before_time, @cur_before_id) AND (t.created_at, t.id) > (@cur_after_time, @cur_after_id)
                                                                    UNION
    SELECT t.created_at, t.id, @comment_tag::int as tag FROM comments t WHERE @comment_tag != 0 AND t.feed_event_id = $1 AND t.deleted = false
        AND (t.created_at, t.id) < (@cur_before_time, @cur_before_id) AND (t.created_at, t.id) > (@cur_after_time, @cur_after_id)
) as interactions

ORDER BY CASE WHEN @paging_forward::bool THEN (created_at, id) END ASC,
         CASE WHEN NOT @paging_forward::bool THEN (created_at, id) END DESC
LIMIT $2;

-- name: CountInteractionsByFeedEventIDBatch :batchmany
SELECT count(*), @admire_tag::int as tag FROM admires t WHERE @admire_tag != 0 AND t.feed_event_id = $1 AND t.deleted = false
                                                        UNION
SELECT count(*), @comment_tag::int as tag FROM comments t WHERE @comment_tag != 0 AND t.feed_event_id = $1 AND t.deleted = false;

-- name: GetAdmireByActorIDAndFeedEventID :batchone
SELECT * FROM admires WHERE actor_id = $1 AND feed_event_id = $2 AND deleted = false;