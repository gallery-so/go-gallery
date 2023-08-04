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

-- name: GetUserLabels :many
select follower id from follows where not deleted group by 1
union
select followee id from follows where not deleted group by 1
union
select user_id id from owned_contracts where displayed group by 1;

-- name: GetContractLabels :many
select user_id, contract_id, displayed
from owned_contracts
where contract_id not in (
  select id from contracts where chain || ':' || address = any(@excluded_contracts::varchar[])
) and displayed;

-- name: GetFeedEntityScores :many
select *
from feed_entity_scores
where 
  (@include_viewer::bool or actor_id != @viewer_id)
  and (@include_posts::bool or feed_entity_type != @post_entity_type)
  and (action != any(@excluded_feed_actions::varchar[]) or action is null);
