-- name: GetUserById :one
SELECT * FROM users WHERE id = $1 AND deleted = false;

-- name: GetUserByIdBatch :batchone
SELECT * FROM users WHERE id = $1 AND deleted = false;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username_idempotent = lower(sqlc.arg(username)) AND deleted = false;

-- name: GetUserByUsernameBatch :batchone
SELECT * FROM users WHERE username_idempotent = lower($1) AND deleted = false;

-- name: GetUserByAddress :one
SELECT * FROM users WHERE sqlc.arg(address)::varchar = ANY(addresses) AND deleted = false;

-- name: GetUserByAddressBatch :batchone
SELECT * FROM users WHERE $1::varchar = ANY(addresses) AND deleted = false;

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

-- name: GetNftById :one
SELECT * FROM nfts WHERE id = $1 AND deleted = false;

-- name: GetNftByIdBatch :batchone
SELECT * FROM nfts WHERE id = $1 AND deleted = false;

-- name: GetNftsByCollectionId :many
SELECT n.* FROM collections c, unnest(c.nfts)
    WITH ORDINALITY AS x(nft_id, nft_ord)
    INNER JOIN nfts n ON n.id = x.nft_id
    WHERE c.id = $1 AND c.deleted = false AND n.deleted = false ORDER BY x.nft_ord;

-- name: GetNftsByCollectionIdBatch :batchmany
SELECT n.* FROM collections c, unnest(c.nfts)
    WITH ORDINALITY AS x(nft_id, nft_ord)
    INNER JOIN nfts n ON n.id = x.nft_id
    WHERE c.id = $1 AND c.deleted = false AND n.deleted = false ORDER BY x.nft_ord;

-- name: GetNftsByOwnerAddress :many
SELECT * FROM nfts WHERE owner_address = $1 AND deleted = false;

-- name: GetNftsByOwnerAddressBatch :batchmany
SELECT * FROM nfts WHERE owner_address = $1 AND deleted = false;

-- name: GetTokensByUserID :many
SELECT * FROM tokens WHERE owner_user_id = $1 AND deleted = false;

-- name: GetTokensByUserIDBatch :batchmany
SELECT * FROM tokens WHERE owner_user_id = $1 AND deleted = false;

-- name: GetTokenByID :one
SELECT * FROM tokens WHERE id = $1 AND deleted = false;

-- name: GetTokenByIDBatch :batchone
SELECT * FROM tokens WHERE id = $1 AND deleted = false;

-- name: GetMembershipByMembershipId :one
SELECT * FROM membership WHERE id = $1 AND deleted = false;

-- name: GetMembershipByMembershipIdBatch :batchone
SELECT * FROM membership WHERE id = $1 AND deleted = false;

-- name: GetWalletByID :one
SELECT * FROM wallets WHERE id = $1 AND deleted = false;

-- name: GetWalletByIDBatch :batchone
SELECT * FROM wallets WHERE id = $1 AND deleted = false;

-- name: GetWalletByAddressDetails :one
SELECT wallets.* FROM wallets WHERE address = $1 AND chain = $2 AND deleted = false;

-- name: GetWalletByAddressDetailsBatch :batchone
SELECT wallets.* FROM wallets WHERE address = $1 AND chain = $2 AND deleted = false;

-- name: GetWalletsByUserID :many
SELECT w.* FROM users u, unnest(u.addresses) WITH ORDINALITY AS a(addr, addr_ord) INNER JOIN wallets w on w.id = a.addr WHERE u.id = $1 AND u.deleted = false AND w.deleted = false ORDER BY a.addr_ord;

-- name: GetWalletsByUserIDBatch :batchmany
SELECT w.* FROM users u, unnest(u.addresses) WITH ORDINALITY AS a(addr, addr_ord) INNER JOIN wallets w on w.id = a.addr WHERE u.id = $1 AND u.deleted = false AND w.deleted = false ORDER BY a.addr_ord;

-- name: GetContractByID :one
select * FROM contracts WHERE id = $1 AND deleted = false;

-- name: GetContractByIDBatch :batchone
select * FROM contracts WHERE id = $1 AND deleted = false;

-- name: GetContractByDetails :one
select * FROM contracts WHERE address = $1 AND chain = $2 AND deleted = false;

-- name: GetContractByDetailsBatch :batchone
select * FROM contracts WHERE address = $1 AND chain = $2 AND deleted = false;
