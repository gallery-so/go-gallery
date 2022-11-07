-- name: CreateAdmire :one
insert into admires (id, feed_event_id, actor_id) values ($1, $2, $3) returning id;

-- name: DeleteAdmireByID :exec
update admires set deleted = true where id = $1;