----------------------
-- Contracts
----------------------

-- Addresses normally get very high weighting (because if a search matches by address, it's
-- almost certainly the result you were looking for). POAP addresses are actually a series of
-- words, though, and should be treated more like descriptions than addresses. As such, we split
-- addresses into two weighting categories: typical unique addresses get weight A, and POAPs
-- get weight D.
alter table contracts add column fts_address tsvector
    generated always as (
        setweight(to_tsvector('simple', coalesce(address,'')), (case when chain != '5' then 'A' else 'D' end)::"char")
    ) stored;

alter table contracts add column fts_name tsvector
    generated always as (
        to_tsvector('simple', coalesce(name,''))
    ) stored;

alter table contracts add column fts_description_english tsvector
    generated always as (
        to_tsvector('english', coalesce(description,''))
    ) stored;

create index contracts_fts_address_idx on contracts using gin (fts_address);
create index contracts_fts_name_idx on contracts using gin (fts_name);
create index contracts_fts_description_english_idx on contracts using gin (fts_description_english);

-- Contract relevance: ranks contracts based on the number of Gallery users who are currently
-- displaying items from each contract. min_count is set to 0, but is kept as a CTE so we can
-- change it easily if we'd like to tweak the baseline relevance score.
-- Score is: (# users displaying at least one piece from contract) / (# users displaying most popular contract),
-- such that all contracts have a score from [0.0, 1.0], and the contract being displayed by the most users has
-- a score of 1.0.
create materialized view contract_relevance as (
    with users_per_contract as (
        select con.id from contracts con, collections col, unnest(col.nfts) as col_token_id
        inner join tokens t on t.id = col_token_id and t.deleted = false
        where t.contract = con.id and col.owner_user_id = t.owner_user_id
        and col.deleted = false and con.deleted = false group by (con.id, t.owner_user_id)
    ),
    min_count as (
        -- The baseline score is 0, because contracts that aren't displayed by anyone on
        -- Gallery are potentially contracts we want to filter out entirely
        select 0 as count
    ),
    max_count as (
        select count(id) from users_per_contract group by id order by count(id) desc limit 1
    )
    select id, (min_count.count + count(users_per_contract.id)) / (min_count.count + max_count.count)::decimal as score
        from users_per_contract, min_count, max_count group by (id, min_count.count, max_count.count)
    union
    -- Null id contains the minimum score
    select null as id, min_count.count / (min_count.count + max_count.count)::decimal as score
        from min_count, max_count
);

create unique index contract_relevance_id_idx on contract_relevance(id);

----------------------
-- Users
----------------------
-- fts_username has special handling for splitting on 0x prefixes and numbers, since those are common
-- in usernames and not handled by Postgres text processing by default
alter table users add column fts_username tsvector
    generated always as (
        to_tsvector('simple', username || case when universal = false then (' ' || regexp_replace(username, '(^0[xX]|\d+|\D+)', '\1 ', 'g')) else '' end)
    ) stored;

alter table users add column fts_bio_english tsvector
    generated always as (
        to_tsvector('english', coalesce(bio, ''))
    ) stored;

create index users_fts_username_idx on users using gin (fts_username);
create index users_fts_bio_english_idx on users using gin (fts_bio_english);

alter table wallets add column fts_address tsvector
    generated always as (
        to_tsvector('simple', address)
    ) stored;

create index wallets_fts_address_idx on wallets using gin (fts_address);

-- User relevance: ranks users based on how many followers they have.
-- Score is: (1 + # followers) / (1 + highest # followers), such that all users have a
-- score from (0.0, 1.0], and the user with the most followers has a 1.0 score.
create materialized view user_relevance as (
    with followers_per_user as (
        select users.id, count(follows) from users, follows
        where follows.followee = users.id and users.deleted = false and follows.deleted = false
        group by users.id
    ),
    min_count as (
        -- The baseline score is 1 follower. Everyone gets at least this much relevance,
        -- even if they don't actually have any followers, because everyone should have
        -- a non-zero relevance score. All of our users are relevant!
        select 1 as count
    ),
    max_count as (
        select followers_per_user.count from followers_per_user order by followers_per_user.count desc limit 1
    )
    select id, (min_count.count + followers_per_user.count) / (min_count.count + max_count.count)::decimal as score
        from followers_per_user, min_count, max_count
    union
    -- Null id contains the minimum score
    select null as id, min_count.count / (min_count.count + max_count.count)::decimal as score
        from min_count, max_count
);

create unique index user_relevance_id_idx on user_relevance(id);

--------------------------
-- Galleries
--------------------------
alter table galleries add column fts_name tsvector
    generated always as (
        to_tsvector('simple', name)
    ) stored;

alter table galleries add column fts_description_english tsvector
    generated always as (
        to_tsvector('english', description)
    ) stored;

create index galleries_fts_name_idx on galleries using gin (fts_name);
create index galleries_fts_description_english_idx on galleries using gin (fts_description_english);

-- Gallery relevance: ranks galleries based on number of views.
-- Score is: (1 + # views) / (1 + highest # views), such that all galleries have a
-- score from (0.0, 1.0], and the gallery with the most views has a 1.0 score.
create materialized view gallery_relevance as (
    with views_per_gallery as (
        select gallery_id, count(gallery_id) from events
            where action = 'ViewedGallery' and deleted = false group by gallery_id
    ),
    max_count as (
        select views_per_gallery.count from views_per_gallery order by views_per_gallery.count desc limit 1
    ),
    min_count as (
        -- The baseline score is 1 view, because every gallery is relevant!
        select 1 as count
    )
    select gallery_id as id, (min_count.count + views_per_gallery.count) / (min_count.count + max_count.count)::decimal as score
        from views_per_gallery, min_count, max_count
    union
    -- Null id contains the minimum score
    select null as id, min_count.count / (min_count.count + max_count.count)::decimal as score
        from min_count, max_count
);

create unique index gallery_relevance_id_idx on gallery_relevance(id);