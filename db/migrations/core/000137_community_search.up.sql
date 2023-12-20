drop materialized view if exists contract_relevance;
alter table contracts drop column fts_address;
alter table contracts drop column fts_description_english;
alter table contracts drop column fts_name;

create materialized view community_relevance as (
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

    users_per_community as (
        select com.id from communities com, collections col, unnest(col.nfts) as col_token_id
        inner join community_tokens t on t.token_id = col_token_id
        where t.community_id = com.id and col.owner_user_id = t.owner_user_id
        and col.deleted = false and com.deleted = false group by (com.id, t.owner_user_id)
    ),

    min_count as (
        -- The baseline score is 0, because communities that aren't displayed by anyone on
        -- Gallery are potentially communities we want to filter out entirely
        select 0 as count
    ),

    max_count as (
        select count(id) from users_per_community group by id order by count(id) desc limit 1
    )

    select id, (min_count.count + count(users_per_community.id)) / (min_count.count + max_count.count)::decimal as score
        from users_per_community, min_count, max_count group by (id, min_count.count, max_count.count)
    union
    -- Null id contains the minimum score
    select null as id, min_count.count / (min_count.count + max_count.count)::decimal as score
        from min_count, max_count
);

create unique index community_relevance_id_idx on community_relevance(id);

-- Addresses normally get very high weighting (because if a search matches by address, it's
-- almost certainly the result you were looking for). POAP addresses are actually a series of
-- words, though, and should be treated more like descriptions than addresses. As such, we split
-- addresses into two weighting categories: typical unique addresses get weight A, and POAPs
-- get weight D.
alter table communities add column fts_community_key tsvector
    generated always as (
        setweight(
            to_tsvector('simple',
            -- Contract communities and Art Blocks communities both use the contract address as key2
            case when (community_type = 0 or community_type = 1) then key2 else '' end
            ),
            -- POAPs get weight D
            (case when (community_type = 0 and key1 = '5') then 'D' else 'A' end)::"char"
        )

        ||

        -- Special handling for community providers: we want to add "prohibition art" as a search phrase for
        -- Prohibition communities, and "art blocks" as a search phrase for other Art Blocks communities.
        -- Give weight C to this kind of tagging.
        case
            when (community_type = 1 and key1 = '1' and key2 = '0x47a91457a3a1f700097199fd63c039c4784384ab') then
                setweight(to_tsvector('simple', 'prohibition art'), 'C')
            when (community_type = 1) then
                setweight(to_tsvector('simple', 'art blocks'), 'C')
            else ''
        end
    ) stored;

alter table communities add column fts_name tsvector
    generated always as (
        to_tsvector('simple', coalesce(override_name,name,''))
    ) stored;

alter table communities add column fts_description_english tsvector
    generated always as (
        to_tsvector('english', coalesce(override_description,description,''))
    ) stored;

create index communities_fts_community_key_idx on communities using gin (fts_community_key);
create index communities_fts_name_idx on communities using gin (fts_name);
create index communities_fts_description_english_idx on communities using gin (fts_description_english);

select cron.schedule('refresh-community-relevance', '15 * * * *', 'refresh materialized view concurrently community_relevance with data');

-- TODO: Manually unschedule the contract_relevance cron job, because it doesn't have a name, and its ID may vary between environments.
-- On prod, run this: select cron.unschedule(5);
