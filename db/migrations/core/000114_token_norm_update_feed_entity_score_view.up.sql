create or replace view feed_entity_score_view as (
  with report_after as (
    select now() - interval '7 day' ts
  ),

  selected_posts as (
      select posts.id, posts.created_at, posts.actor_id, posts.contract_ids, count(distinct comments.id) + count(distinct admires.id) interactions
      from posts
      left join comments on comments.post_id = posts.id
      left join admires on admires.post_id = posts.id
      where posts.created_at >= (select ts from report_after)
        and not posts.deleted
      group by posts.id, posts.created_at, posts.actor_id, posts.contract_ids
  ),

  selected_events as (
      select feed_events.id, feed_events.event_time created_at, feed_events.owner_id actor_id, feed_events.action, count(distinct comments.id) + count(distinct admires.id) interactions
      from feed_events
      left join comments on comments.feed_event_id = feed_events.id
      left join admires on admires.feed_event_id = feed_events.id
      where feed_events.event_time >= (select ts from report_after)
        and not feed_events.deleted
      group by feed_events.id, feed_events.event_time, feed_events.owner_id, feed_events.action
  ),

  event_contracts as (
      select feed_event_id, array_agg(contract_id) contract_ids
      from (
        select feed_events.id feed_event_id, contracts.id contract_id
        from 
          feed_events,
          -- The only event that we currently store with token ids is the 'GalleryUpdated' event which includes the 'gallery_new_token_ids' field
          lateral jsonb_each((data->>'gallery_new_token_ids')::jsonb) x(key, value),
          jsonb_array_elements_text(x.value) tid(id),
          tokens,
          contracts
        where
          feed_events.event_time >= (select ts from report_after)
          and not feed_events.deleted
          and tid.id = tokens.id
          and contracts.id = tokens.contract_id
          and not tokens.deleted
          and not contracts.deleted
        group by feed_events.id, contracts.id
      ) t
      group by feed_event_id
  )

  select id, created_at, actor_id, action, contract_ids, interactions, 0 feed_entity_type, now()::timestamptz as last_updated
  from selected_events
  left join event_contracts on selected_events.id = event_contracts.feed_event_id

  union all

  select id, created_at, actor_id, '', contract_ids, interactions, 1 feed_entity_type, now()::timestamptz as last_updated
  from selected_posts
);
refresh materialized view feed_entity_scores;
