-- name: CreateFeedEventAdmire :one
with feed_event_to_admire as (select id from feed_events where feed_events.id = @feed_event_id and not deleted)
insert into admires (id, feed_event_id, actor_id) (select @id, feed_event_to_admire.id, @actor_id from feed_event_to_admire)
on conflict (actor_id, feed_event_id) where deleted = false do update set last_updated = now()
returning id;

-- name: CreatePostAdmire :one
with post_to_admire as (select id from posts where posts.id = @post_id and not deleted)
insert into admires (id, post_id, actor_id) (select @id, post_to_admire.id, @actor_id from post_to_admire)
on conflict (actor_id, post_id) where deleted = false do update set last_updated = now()
returning id;

-- name: CreateTokenAdmire :one
with token_to_admire as (select id from tokens where tokens.id = @token_id and not deleted)
insert into admires (id, token_id, actor_id) (select @id, token_to_admire.id, @actor_id from token_to_admire)
on conflict (actor_id, token_id) where deleted = false do update set last_updated = now()
returning id;

-- name: CreateCommentAdmire :one
with comment_to_admire as (select id from comments where comments.id = @comment_id and not deleted and not removed)
insert into admires (id, comment_id, actor_id) (select @id, comment_to_admire.id, @actor_id from comment_to_admire)
on conflict (actor_id, comment_id) where deleted = false do update set last_updated = now()
returning id;

-- name: DeleteAdmireByID :exec
update admires set deleted = true where id = $1;
