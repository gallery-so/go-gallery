-- name: GalleryRepoCreate :one
insert into galleries (id, version, collections, owner_user_id) values ($1, $2, $3, $4) returning id;

-- name: GalleryRepoUpdate :execrows
update galleries set last_updated = $1, collections = $2 where id = @gallery_id and owner_user_id = $3;

-- name: GalleryRepoAddCollections :execrows
update galleries set collections = @collection_ids::text[] || collections where id = @gallery_id and owner_user_id = $1;

-- name: GalleryRepoCheckOwnCollections :one
select count(*) from collections where id = any(@collection_ids) and owner_user_id = $1;

-- name: GalleryRepoCountAllCollections :one
select count(*) from collections where owner_user_id = $1 and deleted = false;

-- name: GalleryRepoCountColls :one
select count(c.id) from galleries g, unnest(g.collections) with ordinality as u(coll, coll_ord)
    left join collections c on c.id = coll where g.id = $1 and c.deleted = false and g.deleted = false;

-- name: GalleryRepoGetCollections :many
select id from collections where owner_user_id = $1 and deleted = false;

-- name: GalleryRepoGetGalleryCollections :many
select c.id from galleries g, unnest(g.collections) with ordinality as u(coll, coll_ord)
    left join collections c on c.id = u.coll
    where g.id = $1 and c.deleted = false and g.deleted = false order by u.coll_ord;

-- name: GalleryRepoGetByUserIDRaw :many
select * from galleries g where g.owner_user_id = $1 and g.deleted = false;

-- name: GalleryRepoGetPreviewsForUserID :many
select (t.media ->> 'thumbnail_url')::text from galleries g,
    unnest(g.collections) with ordinality as collection_ids(id, ord) inner join collections c on c.id = collection_ids.id and c.deleted = false,
    unnest(c.nfts) with ordinality as token_ids(id, ord) inner join tokens t on t.id = token_ids.id and t.deleted = false
    where g.owner_user_id = $1 and g.deleted = false and t.media ->> 'thumbnail_url' != ''
    order by collection_ids.ord, token_ids.ord limit $2;