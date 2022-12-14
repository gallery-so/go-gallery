// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.16.0
// source: gallery.sql

package coredb

import (
	"context"
	"database/sql"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

const galleryRepoAddCollections = `-- name: GalleryRepoAddCollections :execrows
update galleries set collections = $2::text[] || collections where id = $3 and owner_user_id = $1
`

type GalleryRepoAddCollectionsParams struct {
	OwnerUserID   persist.DBID
	CollectionIds []string
	GalleryID     persist.DBID
}

func (q *Queries) GalleryRepoAddCollections(ctx context.Context, arg GalleryRepoAddCollectionsParams) (int64, error) {
	result, err := q.db.Exec(ctx, galleryRepoAddCollections, arg.OwnerUserID, arg.CollectionIds, arg.GalleryID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

const galleryRepoCheckOwnCollections = `-- name: GalleryRepoCheckOwnCollections :one
select count(*) from collections where id = any($2) and owner_user_id = $1
`

type GalleryRepoCheckOwnCollectionsParams struct {
	OwnerUserID   persist.DBID
	CollectionIds persist.DBIDList
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
	GalleryID   persist.DBID
	OwnerUserID persist.DBID
	Name        sql.NullString
	Description sql.NullString
	Position    string
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

const galleryRepoDelete = `-- name: GalleryRepoDelete :one
update galleries set deleted = true where id = $2 and owner_user_id = $1 returning id, deleted, last_updated, created_at, version, owner_user_id, collections, name, description, hidden, position
`

type GalleryRepoDeleteParams struct {
	OwnerUserID persist.DBID
	GalleryID   persist.DBID
}

func (q *Queries) GalleryRepoDelete(ctx context.Context, arg GalleryRepoDeleteParams) (Gallery, error) {
	row := q.db.QueryRow(ctx, galleryRepoDelete, arg.OwnerUserID, arg.GalleryID)
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

const galleryRepoEnsureCollsOwnedByUser = `-- name: GalleryRepoEnsureCollsOwnedByUser :exec
update galleries set collections = collections || unused_colls from (select unnest(id) from collections c, galleries g where not c.id = any(g.collections)) as unused_colls where galleries.owner_user_id = $1 and galleries.id = (select id from galleries g where g.owner_user_id = $1 order by position limit 1)
`

func (q *Queries) GalleryRepoEnsureCollsOwnedByUser(ctx context.Context, userID persist.DBID) error {
	_, err := q.db.Exec(ctx, galleryRepoEnsureCollsOwnedByUser, userID)
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
select (t.media ->> 'thumbnail_url')::text from galleries g,
    unnest(g.collections) with ordinality as collection_ids(id, ord) inner join collections c on c.id = collection_ids.id and c.deleted = false,
    unnest(c.nfts) with ordinality as token_ids(id, ord) inner join tokens t on t.id = token_ids.id and t.deleted = false
    where g.owner_user_id = $1 and g.deleted = false and t.media ->> 'thumbnail_url' != ''
    order by collection_ids.ord, token_ids.ord limit $2
`

type GalleryRepoGetPreviewsForUserIDParams struct {
	OwnerUserID persist.DBID
	Limit       int32
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
update galleries set last_updated = $1, collections = $2 where id = $4 and owner_user_id = $3
`

type GalleryRepoUpdateParams struct {
	LastUpdated time.Time
	Collections persist.DBIDList
	OwnerUserID persist.DBID
	GalleryID   persist.DBID
}

func (q *Queries) GalleryRepoUpdate(ctx context.Context, arg GalleryRepoUpdateParams) (int64, error) {
	result, err := q.db.Exec(ctx, galleryRepoUpdate,
		arg.LastUpdated,
		arg.Collections,
		arg.OwnerUserID,
		arg.GalleryID,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}
