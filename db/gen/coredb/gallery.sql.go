// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.18.0
// source: gallery.sql

package coredb

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
)

const galleryRepoAddCollections = `-- name: GalleryRepoAddCollections :execrows
update galleries set last_updated = now(), collections = $1::text[] || collections where galleries.id = $2 and (select count(*) from collections c where c.id = any($1) and c.gallery_id = $2 and c.deleted = false) = coalesce(array_length($1, 1), 0)
`

type GalleryRepoAddCollectionsParams struct {
	CollectionIds []string     `json:"collection_ids"`
	GalleryID     persist.DBID `json:"gallery_id"`
}

func (q *Queries) GalleryRepoAddCollections(ctx context.Context, arg GalleryRepoAddCollectionsParams) (int64, error) {
	result, err := q.db.Exec(ctx, galleryRepoAddCollections, arg.CollectionIds, arg.GalleryID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

const galleryRepoCheckOwnCollections = `-- name: GalleryRepoCheckOwnCollections :one
select count(*) from collections where id = any($2) and owner_user_id = $1
`

type GalleryRepoCheckOwnCollectionsParams struct {
	OwnerUserID   persist.DBID     `json:"owner_user_id"`
	CollectionIds persist.DBIDList `json:"collection_ids"`
}

func (q *Queries) GalleryRepoCheckOwnCollections(ctx context.Context, arg GalleryRepoCheckOwnCollectionsParams) (int64, error) {
	row := q.db.QueryRow(ctx, galleryRepoCheckOwnCollections, arg.OwnerUserID, arg.CollectionIds)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const galleryRepoCountAllCollections = `-- name: GalleryRepoCountAllCollections :one
select count(*) from collections where owner_user_id = $1 and deleted = false
`

func (q *Queries) GalleryRepoCountAllCollections(ctx context.Context, ownerUserID persist.DBID) (int64, error) {
	row := q.db.QueryRow(ctx, galleryRepoCountAllCollections, ownerUserID)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const galleryRepoCountColls = `-- name: GalleryRepoCountColls :one
select count(c.id) from galleries g, unnest(g.collections) with ordinality as u(coll, coll_ord)
    left join collections c on c.id = coll where g.id = $1 and c.deleted = false and g.deleted = false
`

func (q *Queries) GalleryRepoCountColls(ctx context.Context, id persist.DBID) (int64, error) {
	row := q.db.QueryRow(ctx, galleryRepoCountColls, id)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const galleryRepoCreate = `-- name: GalleryRepoCreate :one
insert into galleries (id, owner_user_id, name, description, position) values ($1, $2, $3, $4, $5) returning id, deleted, last_updated, created_at, version, owner_user_id, collections, name, description, hidden, position
`

type GalleryRepoCreateParams struct {
	GalleryID   persist.DBID `json:"gallery_id"`
	OwnerUserID persist.DBID `json:"owner_user_id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Position    string       `json:"position"`
}

func (q *Queries) GalleryRepoCreate(ctx context.Context, arg GalleryRepoCreateParams) (Gallery, error) {
	row := q.db.QueryRow(ctx, galleryRepoCreate,
		arg.GalleryID,
		arg.OwnerUserID,
		arg.Name,
		arg.Description,
		arg.Position,
	)
	var i Gallery
	err := row.Scan(
		&i.ID,
		&i.Deleted,
		&i.LastUpdated,
		&i.CreatedAt,
		&i.Version,
		&i.OwnerUserID,
		&i.Collections,
		&i.Name,
		&i.Description,
		&i.Hidden,
		&i.Position,
	)
	return i, err
}

const galleryRepoDelete = `-- name: GalleryRepoDelete :exec
update galleries set deleted = true where galleries.id = $1 and (select count(*) from galleries g where g.owner_user_id = $2 and g.deleted = false and not g.id = $1) > 0 and not coalesce((select featured_gallery::varchar from users u where u.id = $2), '') = $1
`

type GalleryRepoDeleteParams struct {
	GalleryID   persist.DBID `json:"gallery_id"`
	OwnerUserID persist.DBID `json:"owner_user_id"`
}

func (q *Queries) GalleryRepoDelete(ctx context.Context, arg GalleryRepoDeleteParams) error {
	_, err := q.db.Exec(ctx, galleryRepoDelete, arg.GalleryID, arg.OwnerUserID)
	return err
}

const galleryRepoGetByUserIDRaw = `-- name: GalleryRepoGetByUserIDRaw :many
select id, deleted, last_updated, created_at, version, owner_user_id, collections, name, description, hidden, position from galleries g where g.owner_user_id = $1 and g.deleted = false order by position
`

func (q *Queries) GalleryRepoGetByUserIDRaw(ctx context.Context, ownerUserID persist.DBID) ([]Gallery, error) {
	rows, err := q.db.Query(ctx, galleryRepoGetByUserIDRaw, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Gallery
	for rows.Next() {
		var i Gallery
		if err := rows.Scan(
			&i.ID,
			&i.Deleted,
			&i.LastUpdated,
			&i.CreatedAt,
			&i.Version,
			&i.OwnerUserID,
			&i.Collections,
			&i.Name,
			&i.Description,
			&i.Hidden,
			&i.Position,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const galleryRepoGetGalleryCollections = `-- name: GalleryRepoGetGalleryCollections :many
select c.id from galleries g, unnest(g.collections) with ordinality as u(coll, coll_ord)
    left join collections c on c.id = u.coll
    where g.id = $1 and c.deleted = false and g.deleted = false order by u.coll_ord
`

func (q *Queries) GalleryRepoGetGalleryCollections(ctx context.Context, id persist.DBID) ([]persist.DBID, error) {
	rows, err := q.db.Query(ctx, galleryRepoGetGalleryCollections, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []persist.DBID
	for rows.Next() {
		var id persist.DBID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		items = append(items, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const galleryRepoGetPreviewsForUserID = `-- name: GalleryRepoGetPreviewsForUserID :many
select (tm.media ->> 'thumbnail_url')::text from galleries g,
    unnest(g.collections) with ordinality as collection_ids(id, ord) inner join collections c on c.id = collection_ids.id and c.deleted = false,
    unnest(c.nfts) with ordinality as token_ids(id, ord) inner join tokens t on t.id = token_ids.id and t.displayable and t.deleted = false
    inner join token_medias tm on t.token_media_id = tm.id and tm.media ->> 'thumbnail_url' != ''
    where g.owner_user_id = $1 and g.deleted = false
    order by collection_ids.ord, token_ids.ord limit $2
`

type GalleryRepoGetPreviewsForUserIDParams struct {
	OwnerUserID persist.DBID `json:"owner_user_id"`
	Limit       int32        `json:"limit"`
}

func (q *Queries) GalleryRepoGetPreviewsForUserID(ctx context.Context, arg GalleryRepoGetPreviewsForUserIDParams) ([]string, error) {
	rows, err := q.db.Query(ctx, galleryRepoGetPreviewsForUserID, arg.OwnerUserID, arg.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []string
	for rows.Next() {
		var column_1 string
		if err := rows.Scan(&column_1); err != nil {
			return nil, err
		}
		items = append(items, column_1)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const galleryRepoUpdate = `-- name: GalleryRepoUpdate :execrows
update galleries set last_updated = now(), collections = $1 where galleries.id = $2 and (select count(*) from collections c where c.id = any($1) and c.gallery_id = $2 and c.deleted = false) = coalesce(array_length($1, 1), 0)
`

type GalleryRepoUpdateParams struct {
	CollectionIds persist.DBIDList `json:"collection_ids"`
	GalleryID     persist.DBID     `json:"gallery_id"`
}

func (q *Queries) GalleryRepoUpdate(ctx context.Context, arg GalleryRepoUpdateParams) (int64, error) {
	result, err := q.db.Exec(ctx, galleryRepoUpdate, arg.CollectionIds, arg.GalleryID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}
