-- name: CreateAdmire :one
INSERT INTO admires (id, feed_event_id, post_id, token_id, actor_id)
VALUES ($1, sqlc.narg('feed_event'), sqlc.narg('post'), sqlc.narg('token'), $2)
ON CONFLICT (actor_id, token_id) WHERE deleted = false DO NOTHING
RETURNING id;

-- name: DeleteAdmireByID :exec
update admires set deleted = true where id = $1;
