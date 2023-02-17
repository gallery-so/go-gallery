-- name: GetFollowGraphSource :many
select
	follows.follower,
	follows.followee
from
	follows,
	users as followers,
	users as followees,
  (
    select owner_user_id
    from collections
    where cardinality(nfts) > 0 and deleted = false
    group by owner_user_id
  ) displaying
where
	follows.follower = followers.id
  and follows.followee = displaying.owner_user_id
	and followers.deleted is false
	and follows.followee = followees.id
	and followees.deleted is false
	and follows.deleted = false;

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
