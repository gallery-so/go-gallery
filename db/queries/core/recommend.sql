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

-- name: FeedEntityScoring :many
with ids as (
    select *
    from feed_entities fe
    where 
      fe.created_at >= @window_end
      and (@include_viewer::bool or fe.actor_id != @viewer_id::varchar)
      and (@include_posts::bool or feed_entity_type != @post_entity_type)
), selected_posts as (
    select
      ids.id,
      ids.feed_entity_type,
      ids.created_at,
      p.actor_id,
      p.contract_ids,
      count(distinct c.id) + count(distinct a.id) interactions
    from ids
    join posts p on p.id = ids.id and feed_entity_type = @post_entity_type
    left join comments c on c.post_id = ids.id
    left join admires a on a.post_id = ids.id
    group by ids.id, ids.feed_entity_type, ids.created_at, p.actor_id, p.contract_ids
), feed_event_contract_ids as (
    select t.feed_event_id feed_event_id, array_agg(t.contract_id) contract_ids
    from (
      select f.id feed_event_id, c.id contract_id
      from ids,
        feed_events f,
        lateral jsonb_each((data->>'gallery_new_token_ids')::jsonb) x(key, value),
        jsonb_array_elements_text(x.value) tid(id),
        tokens t,
        contracts c
      where
        ids.id = f.id
        and feed_entity_type = @feed_event_entity_type
        and tid.id = t.id
        and c.id = t.contract
        and not t.deleted
        and not c.deleted
      group by 1, 2
    ) t
    group by 1
), selected_events as (
    select
      ids.id,
      ids.feed_entity_type,
      ids.created_at,
      ids.actor_id,
      feed_event_contract_ids.contract_ids,
      count(distinct c.id) + count(distinct a.id) interactions
    from ids
    join feed_events e on e.id = ids.id and feed_entity_type = @feed_event_entity_type
    left join comments c on c.feed_event_id = ids.id
    left join admires a on a.feed_event_id = ids.id
    left join feed_event_contract_ids on feed_event_contract_ids.feed_event_id = ids.id
    where not action = any(@excluded_feed_actions::varchar[])
    group by ids.id, ids.feed_entity_type, ids.created_at, ids.actor_id, feed_event_contract_ids.contract_ids
)
select * from selected_posts
union all
select * from selected_events;
