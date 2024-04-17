-- name: GalleryRepoCreate :one
insert into galleries (id, owner_user_id, name, description, position) values (@gallery_id, @owner_user_id, @name, @description, @position) returning *;

-- name: GalleryRepoUpdate :execrows
update galleries set last_updated = now(), collections = @collection_ids where galleries.id = @gallery_id and (select count(*) from collections c where c.id = any(@collection_ids) and c.gallery_id = @gallery_id and c.deleted = false) = coalesce(array_length(@collection_ids, 1), 0);

-- name: GalleryRepoAddCollections :execrows
update galleries set last_updated = now(), collections = @collection_ids::text[] || collections where galleries.id = @gallery_id and (select count(*) from collections c where c.id = any(@collection_ids) and c.gallery_id = @gallery_id and c.deleted = false) = coalesce(array_length(@collection_ids, 1), 0);

-- name: GalleryRepoCheckOwnCollections :one
select count(*) from collections where id = any(@collection_ids::dbid[]) and owner_user_id = $1;

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
select coalesce(nullif(tm.media->>'thumbnail_url', ''), nullif(tm.media->>'media_url', ''))::varchar as thumbnail_url from galleries g,
    unnest(g.collections) with ordinality as collection_ids(id, ord) inner join collections c on c.id = collection_ids.id and c.deleted = false,
    unnest(c.nfts) with ordinality as token_ids(id, ord) inner join tokens t on t.id = token_ids.id and t.displayable and t.deleted = false
    inner join token_medias tm on t.token_media_id = tm.id and (tm.media ->> 'thumbnail_url' != '' or tm.media ->> 'media_url' != '')
    where g.owner_user_id = $1 and g.deleted = false
    order by collection_ids.ord, token_ids.ord limit $2;

-- name: GalleryRepoDelete :exec
update galleries set deleted = true where galleries.id = @gallery_id and (select count(*) from galleries g where g.owner_user_id = @owner_user_id and g.deleted = false and not g.id = @gallery_id) > 0 and not coalesce((select featured_gallery::varchar from users u where u.id = @owner_user_id), '') = @gallery_id;
