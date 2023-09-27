-- name: GetFollowGraphSource :many
select
  follows.follower,
  follows.followee
from
  follows,
  users as followers,
  users as followees
where
  follows.follower = followers.id
  and followers.deleted is false
  and follows.followee = followees.id
  and followees.deleted is false
  and follows.deleted = false;

-- name: GetExternalFollowGraphSource :many
select
  external_social_connections.follower_id,
  external_social_connections.followee_id
from
  external_social_connections,
  users as followers,
  users as followees
where
  external_social_connections.follower_id = followers.id
  and followers.deleted is false
  and external_social_connections.followee_id = followees.id
  and followees.deleted is false
  and external_social_connections.deleted = false;

-- name: UpdatedRecommendationResults :exec
insert into recommendation_results
(
  id
  , user_id
  , recommended_user_id
  , recommended_count
) (
  select
    unnest(@id::varchar[])
    , unnest(@user_id::varchar[])
    , unnest(@recommended_user_id::varchar[])
    , unnest(@recommended_count::int[])
)
on conflict (user_id, recommended_user_id, version) where deleted = false
do update set
  recommended_count = recommendation_results.recommended_count + excluded.recommended_count,
  last_updated = now();

-- name: GetTopRecommendedUserIDs :many
select recommended_user_id from top_recommended_users;

-- name: GetFollowEdgesByUserID :many
select * from follows f where f.follower = $1 and f.deleted = false;

-- name: GetUserLabels :many
select follower id from follows where not deleted group by 1
union
select followee id from follows where not deleted group by 1
union
select follower_id id from external_social_connections where not deleted group by 1
union
select followee_id id from external_social_connections where not deleted group by 1
union
select user_id id from owned_contracts where displayed group by 1;

-- name: GetContractLabels :many
select user_id, contract_id, displayed
from owned_contracts
where contract_id not in (
  select id from contracts where chain || ':' || address = any(@excluded_contracts::varchar[])
) and displayed;

-- name: GetFeedEntityScores :many
with refreshed as (
  select greatest((select last_updated from feed_entity_scores limit 1), @window_end::timestamptz) last_updated
)
select sqlc.embed(feed_entity_scores), sqlc.embed(posts)
from feed_entity_scores
join posts on feed_entity_scores.id = posts.id
where feed_entity_scores.created_at > @window_end::timestamptz
  and (@include_viewer::bool or feed_entity_scores.actor_id != @viewer_id)
  and feed_entity_scores.feed_entity_type == @post_entity_type
  and not posts.deleted
union
select sqlc.embed(feed_entity_scores), sqlc.embed(posts)
from feed_entity_score_view feed_entity_scores
join posts on feed_entity_score_view.id = posts.id
where created_at > (select last_updated from refreshed limit 1)
and (@include_viewer::bool or feed_entity_score_view.actor_id != @viewer_id)
and feed_entity_score_view.feed_entity_type == @post_entity_type;
