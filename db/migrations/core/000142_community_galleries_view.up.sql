create materialized view community_galleries as (
    with community_tokens as (
        select c.id as community_id, t.id as token_id, t.token_definition_id, t.owner_user_id as owner_user_id
            from communities c, tokens t
            where c.community_type = 0
                and t.contract_id = c.contract_id
                and not c.deleted
                and not t.deleted
                and t.displayable

        union all

        select c.id as community_id, t.id as token_id, t.token_definition_id, t.owner_user_id as owner_user_id
            from communities c, tokens t, token_community_memberships tcm
            where c.community_type != 0
                and t.token_definition_id = tcm.token_definition_id
                and tcm.community_id = c.id
                and not c.deleted
                and not t.deleted
                and not tcm.deleted
                and t.displayable
    ),

    gallery_tokens as (
        select
            users.id as user_id,
            galleries.id as gallery_id,
            gallery_relevance.score as gallery_relevance,
            ct.community_id,
            ct.token_id,
            ct.token_definition_id,
            tm.media as token_media,
            (galleries.position, array_position(galleries.collections, collections.id), array_position(collections.nfts, ct.token_id)) as position
        from users, galleries
            join gallery_relevance on gallery_relevance.id = galleries.id,
            collections, community_tokens ct
            join token_definitions td on td.id = ct.token_definition_id
            -- This is an inner join, which means a token without a preview won't show up.
            -- We may want to reconsider this in the future!
            join token_medias tm on tm.id = td.token_media_id
        where
            users.universal = false
            and galleries.owner_user_id = users.id
            and collections.owner_user_id = users.id
            and collections.gallery_id = galleries.id
            and ct.owner_user_id = users.id
            and ct.token_id = any(collections.nfts)
            and users.deleted = false
            and galleries.deleted = false
            and collections.deleted = false
            and td.deleted = false
            and tm.deleted = false
        group by
            users.id,
            ct.community_id,
            galleries.id,
            ct.token_id,
            ct.token_definition_id,
            tm.media,
            gallery_relevance.score,
            (galleries.id, array_position(galleries.collections, collections.id), array_position(collections.nfts, ct.token_id))
    )

    -- Get ordered tokens for each gallery, not specific to any community
    select x.user_id,
        null as community_id,
        x.gallery_id,
        x.gallery_relevance,
        array_agg(x.token_id order by x.position) as token_ids,
        array_agg(x.token_definition_id order by x.position) as token_definition_ids,
        array_agg(x.token_media order by x.position) as token_medias
    from
        (select user_id, gallery_id, token_id, token_definition_id, token_media, position, gallery_relevance from gallery_tokens
            group by user_id, gallery_id, token_id, token_definition_id, token_media, position, gallery_relevance) as x
    group by x.user_id, x.gallery_id, x.gallery_relevance

    union all

    -- Get ordered tokens for each gallery, specific to a community
    select user_id,
        community_id,
        gallery_id,
        gallery_relevance,
        array_agg(token_id order by position) as token_ids,
        array_agg(token_definition_id order by position) as token_definition_ids,
        array_agg(token_media order by position) as token_medias from gallery_tokens
    group by user_id, community_id, gallery_id, gallery_relevance
);

create index community_galleries_gallery_id_idx on community_galleries (gallery_id);
create unique index community_galleries_community_id_gallery_id_idx on community_galleries (community_id, gallery_id);
create index community_galleries_community_id_gallery_relevance_idx on community_galleries (community_id, gallery_relevance, gallery_id);

select cron.schedule('refresh-community-galleries', '45 */3 * * *', 'refresh materialized view concurrently community_galleries with data');
