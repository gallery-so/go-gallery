-- name: CreateAdmire :one
insert into admires (id, feed_event_id, post_id, actor_id) values ($1, sqlc.narg('feed_event'), sqlc.narg('post'), $2) returning id;

-- name: DeleteAdmireByID :exec
update admires set deleted = true where id = $1;