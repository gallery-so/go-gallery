-- name: GalleryRepoCreate :one
insert into galleries (id, owner_user_id, name, description, position) values (@gallery_id, @owner_user_id, @name, @description, @position) returning *;

-- name: GalleryRepoUpdate :execrows
update galleries set last_updated = now(), collections = @collection_ids where galleries.id = @gallery_id and (select count(*) from collections c where c.id = any(@collection_ids) and c.gallery_id = @gallery_id and c.deleted = false) = array_length(@collection_ids, 1);

-- name: GalleryRepoAddCollections :execrows
update galleries set last_updated = now(), collections = @collection_ids::text[] || collections where galleries.id = @gallery_id and (select count(*) from collections c where c.id = any(@collection_ids) and c.gallery_id = @gallery_id and c.deleted = false) = array_length(@collection_ids, 1);

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

-- name: GalleryRepoDelete :exec
update galleries set galleries.deleted = true where galleries.id = @gallery_id and (select count(*) from galleries g where g.owner_user_id = @owner_user_id and g.deleted = false and not g.id = @gallery_id) > 0 and not (select featured_gallery from users u where u.id = @owner_user_id) = @gallery_id;

-- name: GalleryRepoEnsureCollsOwnedByUser :exec
update galleries set collections = collections || unused_colls.colls from (select array_agg(c.id)::varchar[] as colls from collections c, galleries g where not c.id = any(g.collections) and g.owner_user_id = @user_id and c.owner_user_id = @user_id and c.deleted = false and g.deleted = false) as unused_colls where galleries.owner_user_id = @user_id and galleries.id = (select g.id from galleries g where g.owner_user_id = @user_id order by g.position limit 1); -- should this be their first gallery or their featured gallery