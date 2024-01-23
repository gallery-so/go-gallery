create materialized view gallery_previews as (
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
            ct.community_id,
            ct.token_id,
            ct.token_definition_id,
            tm.media as token_media,
            (galleries.position, array_position(galleries.collections, collections.id), array_position(collections.nfts, ct.token_id)) as position
        from users, galleries, collections, community_tokens ct
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
            (galleries.id, array_position(galleries.collections, collections.id), array_position(collections.nfts, ct.token_id))
    )

    -- Get ordered tokens for each gallery, not specific to any community
    select x.user_id,
        null as community_id,
        x.gallery_id,
        array_agg(x.token_id order by x.position) as token_ids,
        array_agg(x.token_definition_id order by x.position) as token_definition_ids,
        array_agg(x.token_media order by x.position) as token_medias
    from
        (select user_id, gallery_id, token_id, token_definition_id, token_media, position from gallery_tokens
            group by user_id, gallery_id, token_id, token_definition_id, token_media, position) as x
    group by user_id, gallery_id

    union all

    -- Get ordered tokens for each gallery, specific to a community
    select user_id,
        community_id,
        gallery_id,
        array_agg(token_id order by position) as token_ids,
        array_agg(token_definition_id order by position) as token_definition_ids,
        array_agg(token_media order by position) as token_medias from gallery_tokens
    group by user_id, community_id, gallery_id
);

create index gallery_previews_gallery_id_idx on gallery_previews(gallery_id);
create unique index gallery_previews_community_id_gallery_id_idx on gallery_previews(community_id, gallery_id);

select cron.schedule('refresh-gallery-previews', '45 */3 * * *', 'refresh materialized view concurrently gallery_previews with data');
