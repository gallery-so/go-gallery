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
with refreshed      as ( select greatest((select last_updated from feed_entity_scores limit 1), @window_end) last_updated )
     , gallery_user as ( select id from users where username_idempotent = 'gallery' and not deleted and not universal )
     , t0           as ( select * from feed_entity_scores where feed_entity_scores.created_at > @window_end )
     , t1           as ( select * from feed_entity_score_view where created_at > (select last_updated from refreshed limit 1) )
     , t2           as ( select * from t0 union select * from t1 )
     , t3           as ( select *, (case when lag(created_at) over (partition by actor_id order by created_at desc) is null then 0
                                         when extract(epoch from created_at - lag(created_at) over (partition by actor_id order by created_at desc)) > -(@span::int) then 0
                                         else 1 end)::int cume from t2 )
     , t4           as ( select t3.id, (sum(cume) over (partition by actor_id order by created_at desc))::int group_number from t3 )
select
  sqlc.embed(feed_entity_scores)
  , sqlc.embed(p)
  , row_number() over (partition by p.actor_id order by (t4.group_number, random() > 0.5)) streak
  , coalesce(p.actor_id = (select id from gallery_user), false)::bool is_gallery_post
from t2 feed_entity_scores
join t4 using(id)
join posts p on feed_entity_scores.id = p.id and not p.deleted
left join feed_blocklist fb on p.actor_id = fb.user_id and not fb.deleted and fb.active
where (fb.user_id is null or @viewer_id = fb.user_id);
