-- name: SearchUsers :many
with min_content_score as (
    select score from user_relevance where id is null
)
select u.* from users u left join user_relevance on u.id = user_relevance.id,
    unnest(u.wallets) as wallet_id left join wallets w on w.id = wallet_id and w.deleted = false,
    to_tsquery('simple', websearch_to_tsquery('simple', @query)::text || ':*') simple_partial_query,
    websearch_to_tsquery('simple', @query) simple_full_query,
    websearch_to_tsquery('english', @query) english_full_query,
    min_content_score,
    greatest(
        ts_rank_cd(concat('{', @username_weight::float4, ', 1, 1, 1}')::float4[], u.fts_username, simple_partial_query, 1),
        ts_rank_cd(concat('{', @bio_weight::float4, ', 1, 1, 1}')::float4[], u.fts_bio_english, english_full_query, 1),
        ts_rank_cd('{1, 1, 1, 1}', w.fts_address, simple_full_query) * 1000000000
        ) as match_score,
    coalesce(user_relevance.score, min_content_score.score) as content_score
where (
    simple_partial_query @@ u.fts_username or
    english_full_query @@ u.fts_bio_english or
    simple_full_query @@ w.fts_address
    )
    and u.universal = false and u.deleted = false
group by (u.id, content_score * match_score, content_score, match_score)
order by content_score * match_score desc, content_score desc, match_score desc, length(u.username_idempotent) asc
limit sqlc.arg('limit');

-- name: SearchGalleries :many
with min_content_score as (
    select score from gallery_relevance where id is null
)
select galleries.* from galleries left join gallery_relevance on gallery_relevance.id = galleries.id,
    to_tsquery('simple', websearch_to_tsquery('simple', @query)::text || ':*') simple_partial_query,
    websearch_to_tsquery('english', @query) english_full_query,
    min_content_score,
    greatest(
        ts_rank_cd(concat('{', @name_weight::float4, ', 1, 1, 1}')::float4[], fts_name, simple_partial_query, 1),
        ts_rank_cd(concat('{', @description_weight::float4, ', 1, 1, 1}')::float4[], fts_description_english, english_full_query, 1)
        ) as match_score,
    coalesce(gallery_relevance.score, min_content_score.score) as content_score
where (
    simple_partial_query @@ fts_name or
    english_full_query @@ fts_description_english
    )
    and deleted = false and hidden = false
order by content_score * match_score desc, content_score desc, match_score desc
limit sqlc.arg('limit');

-- name: SearchContracts :many
with min_content_score as (
    select score from contract_relevance where id is null
),
poap_weight as (
    -- Using a CTE as a workaround because sqlc has trouble with this as an inline value
    -- in the ts_rank_cd statement below. We want non-POAP addresses to get crazy high weighting,
    -- but ts_rank weights have to be in the [0, 1] range, so we divide the POAP weight by 1000000000
    -- to offset the fact that we're going to multiply all addresses by 1000000000.
    select sqlc.arg(poap_address_weight)::float4 / 1000000000 as weight
)
select contracts.* from contracts left join contract_relevance on contract_relevance.id = contracts.id,
     to_tsquery('simple', websearch_to_tsquery('simple', @query)::text || ':*') simple_partial_query,
     websearch_to_tsquery('simple', @query) simple_full_query,
     websearch_to_tsquery('english', @query) english_full_query,
     min_content_score,
     poap_weight,
     greatest (
        ts_rank_cd(concat('{', @name_weight::float4, ', 1, 1, 1}')::float4[], fts_name, simple_partial_query, 1),
        ts_rank_cd(concat('{', @description_weight::float4, ', 1, 1, 1}')::float4[], fts_description_english, english_full_query, 1),
        ts_rank_cd(concat('{', poap_weight.weight::float4, ', 1, 1, 1}')::float4[], fts_address, simple_full_query, 1) * 1000000000
        ) as match_score,
     coalesce(contract_relevance.score, min_content_score.score) as content_score
where (
    simple_full_query @@ fts_address or
    simple_partial_query @@ fts_name or
    english_full_query @@ fts_description_english
    )
    and contracts.deleted = false
order by content_score * match_score desc, content_score desc, match_score desc
limit sqlc.arg('limit');



-- name: SearchUsers :many
with min_content_score as (
    select score from user_relevance where id is null
)
select headline, u.* from users u left join user_relevance on u.id = user_relevance.id,
    unnest(u.wallets) as wallet_id left join wallets w on w.id = wallet_id and w.deleted = false,
    to_tsquery('simple', websearch_to_tsquery('simple', @query)::text || ':*') simple_partial_query,
    websearch_to_tsquery('simple', @query) simple_full_query,
    websearch_to_tsquery('english', @query) english_full_query,
    ts_headline('english', u.bio, english_full_query) headline,
    min_content_score,
    greatest(
        ts_rank_cd(concat('{', @username_weight::float4, ', 1, 1, 1}')::float4[], u.fts_username, simple_partial_query, 1),
        ts_rank_cd(concat('{', @bio_weight::float4, ', 1, 1, 1}')::float4[], u.fts_bio_english, english_full_query, 1),
        ts_rank_cd('{1, 1, 1, 1}', w.fts_address, simple_full_query) * 1000000000
        ) as match_score,
    coalesce(user_relevance.score, min_content_score.score) as content_score
where (
    simple_partial_query @@ u.fts_username or
    english_full_query @@ u.fts_bio_english or
    simple_full_query @@ w.fts_address
    )
    and u.universal = false and u.deleted = false
group by (headline, u.id, content_score * match_score, content_score, match_score)
order by content_score * match_score desc, content_score desc, match_score desc, length(u.username_idempotent) asc
limit sqlc.arg('limit');
