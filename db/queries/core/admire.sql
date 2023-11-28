-- name: CreateFeedEventAdmire :one
insert into admires (id, feed_event_id, actor_id) values (@id, @feed_event_id, @actor_id)
on conflict (actor_id, feed_event_id) where deleted = false do update set last_updated = now() returning id;

-- name: CreatePostAdmire :one
insert into admires (id, post_id, actor_id) values (@id, @post_id, @actor_id)
on conflict (actor_id, post_id) where deleted = false do update set last_updated = now() returning id;

-- name: CreateTokenAdmire :one
insert into admires (id, token_id, actor_id) (select @id, @token_id, @actor_id)
on conflict (actor_id, token_id) where deleted = false do update set last_updated = now() returning id;

-- name: CreateCommentAdmire :one
insert into admires (id, comment_id, actor_id) values (@id, @comment_id, @actor_id)
on conflict (actor_id, comment_id) where deleted = false do update set last_updated = now() returning id;

-- name: DeleteAdmireByID :exec
update admires set deleted = true where id = $1;
