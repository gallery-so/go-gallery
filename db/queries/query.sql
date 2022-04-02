-- Notes for writing queries:
--  * A comment between the -- name: section and the query will be added as a comment on the generated method.
--  * The first use of a parameter in a query will determine its generated type, so ordering can be helpful
--    when the type can be inferred from one column but not another (e.g. $1 = c.id AND $1 = ANY(something)).

-- name: GetCollectionById :one
SELECT * FROM collections WHERE id = $1 AND deleted = false;

-- name: GetCollectionsByGalleryId :many
SELECT c.* FROM galleries g, unnest(g.collections)
    WITH ORDINALITY AS x(coll_id, coll_ord)
    LEFT JOIN collections c ON c.id = x.coll_id
    WHERE g.id = $1 AND g.deleted = false AND c.deleted = false ORDER BY x.coll_ord;

-- name: GetGalleryById :one
SELECT * FROM galleries WHERE id = $1 AND deleted = false;

-- name: GetGalleryByCollectionId :one
SELECT g.* FROM galleries g, collections c WHERE c.id = $1 AND c.deleted = false AND $1 = ANY(g.collections) AND g.deleted = false;

-- name: GetGalleriesByUserId :many
SELECT * FROM galleries WHERE owner_user_id = $1 AND deleted = false;

-- name: GetNftById :one
SELECT * FROM nfts WHERE id = $1 AND deleted = false;

-- name: GetNftsByCollectionId :many
SELECT n.* FROM collections c, unnest(c.nfts) WITH ORDINALITY AS x(nft_id, nft_ord) LEFT JOIN nfts n ON n.id = x.nft_id WHERE c.id = $1 AND c.deleted = false AND n.deleted = false ORDER BY x.nft_ord;

-- name: GetNftsByOwnerAddress :many
SELECT * FROM nfts WHERE owner_address = $1 AND deleted = false;

-- name: GetUserById :one
SELECT * FROM users WHERE id = $1 AND deleted = false;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username_idempotent = lower(sqlc.arg(username)) AND deleted = false;

-- name: GetUserByAddress :one
SELECT * FROM users WHERE sqlc.arg(address)::varchar = ANY(addresses) AND deleted = false;
-- ^ Casting to ::varchar above isn't necessary for the query, but forces sqlc to use a string parameter instead of an interface{}