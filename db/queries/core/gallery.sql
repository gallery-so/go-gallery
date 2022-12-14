-- name: GalleryRepoCreate :one
insert into galleries (id, owner_user_id, name, description, position) values (@id, @owner_user_id, @name, @description, @position) returning *;

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

-- name: GalleryRepoGetGalleryCollections :many
select c.id from galleries g, unnest(g.collections) with ordinality as u(coll, coll_ord)
    left join collections c on c.id = u.coll
    where g.id = $1 and c.deleted = false and g.deleted = false order by u.coll_ord;

-- name: GalleryRepoGetByUserIDRaw :many
select * from galleries g where g.owner_user_id = $1 and g.deleted = false order by position;

-- name: GalleryRepoGetPreviewsForUserID :many
select (t.media ->> 'thumbnail_url')::text from galleries g,
    unnest(g.collections) with ordinality as collection_ids(id, ord) inner join collections c on c.id = collection_ids.id and c.deleted = false,
    unnest(c.nfts) with ordinality as token_ids(id, ord) inner join tokens t on t.id = token_ids.id and t.deleted = false
    where g.owner_user_id = $1 and g.deleted = false and t.media ->> 'thumbnail_url' != ''
    order by collection_ids.ord, token_ids.ord limit $2;

-- name: GalleryRepoDelete :one
update galleries set deleted = true where id = @gallery_id and owner_user_id = $1 returning *;

-- name: GalleryRepoEnsureCollsOwnedByUser :exec
update galleries set collections = collections || unused_colls from (select unnest(id) from collections c, galleries g where not c.id = any(g.collections)) as unused_colls where galleries.owner_user_id = @user_id and galleries.id = (select id from galleries g where g.owner_user_id = @user_id order by position limit 1); -- should this be their first gallery or their featured gallery