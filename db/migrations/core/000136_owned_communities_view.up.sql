create materialized view owned_communities as (
    with community_tokens as (
        select c.id as community_id, t.id as token_id, t.owner_user_id as owner_user_id
        from communities c, tokens t
        where c.community_type = 0
            and t.contract_id = c.contract_id
            and not c.deleted
            and not t.deleted

    union all

    select c.id as community_id, t.id as token_id, t.owner_user_id as owner_user_id
        from communities c, tokens t, token_community_memberships tcm
        where c.community_type != 0
            and t.token_definition_id = tcm.token_definition_id
            and tcm.community_id = c.id
            and not c.deleted
            and not t.deleted
            and not tcm.deleted
    ),

    owned_communities as (
    select
      users.id as user_id,
      users.created_at as user_created_at,
      community_tokens.community_id as community_id,
      count(tokens.id) as owned_count
    from users
    join tokens on
      tokens.deleted = false
      and users.id = tokens.owner_user_id
      and coalesce(tokens.is_user_marked_spam, false) = false
    join community_tokens on
      tokens.id = community_tokens.token_id
    where
      users.deleted = false
      and users.universal = false
    group by
      users.id,
      community_tokens.community_id
  ),
  displayed_tokens as (
      select
        owned_communities.user_id,
        owned_communities.community_id,
        community_tokens.token_id
      from owned_communities, galleries, collections, community_tokens
      where
        galleries.deleted = false
        and collections.deleted = false
        and galleries.owner_user_id = owned_communities.user_id
        and collections.owner_user_id = owned_communities.user_id
        and community_tokens.owner_user_id = owned_communities.user_id
        and community_tokens.community_id = owned_communities.community_id
        and community_tokens.token_id = any(collections.nfts)
      group by
        owned_communities.user_id,
        owned_communities.community_id,
        community_tokens.token_id
  ),
  displayed_communities as (
    select user_id, community_id, count(token_id) as displayed_count from displayed_tokens
    group by
      user_id,
      community_id
  )
  select
      owned_communities.user_id,
      owned_communities.user_created_at,
      owned_communities.community_id,
      owned_communities.owned_count,
      coalesce(displayed_communities.displayed_count, 0) as displayed_count,
      (displayed_communities.displayed_count is not null)::bool as displayed,
      now()::timestamptz as last_updated
  from owned_communities
    left join displayed_communities on
      displayed_communities.user_id = owned_communities.user_id and
      displayed_communities.community_id = owned_communities.community_id
);

create unique index owned_communities_user_community_idx on owned_communities(user_id, community_id);
create index owned_communities_user_displayed_idx on owned_communities(user_id, displayed);
create index owned_communities_user_created_at_idx on owned_communities(user_created_at);

select cron.schedule('refresh-owned-communities', '30 */3 * * *', 'refresh materialized view concurrently owned_communities with data');
