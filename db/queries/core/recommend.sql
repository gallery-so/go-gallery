-- name: GetFollowGraphSource :many
select
	follows.follower,
	follows.followee
from
	follows,
	users as followers,
	users as followees,
	-- only recommend users that have content displayed
	(
		select
			owner_user_id
		from collections
		where
			cardinality(nfts) > 0
		and hidden is false
		and deleted is false group by owner_user_id) displayed
where
	follows.follower = followers.id
	and followers.deleted is false
	and follows.followee = followees.id
	and followees.deleted is false
	and follows.deleted = false
	and followees.id = displayed.owner_user_id;

-- name: UpdatedRecommendationResults :exec
insert into recommendation_results
(
  id
  , user_id
  , recommended_user_id
) (
  select
    unnest(@id::varchar[])
    , unnest(@user_id::varchar[])
    , unnest(@recommended_user_id::varchar[])
)
on conflict (user_id, recommended_user_id, version) where deleted = false
do update set
  recommended_count = recommendation_results.recommended_count + 1,
  last_updated = now();

-- name: GetTopRecommendedUserIDs :many
select recommended_user_id from top_recommended_users;

-- name: GetFollowEdgesByUserID :many
select f.followee, f.last_updated from follows f where f.follower = $1 and f.deleted = false;
