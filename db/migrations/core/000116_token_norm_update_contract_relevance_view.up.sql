drop materialized view if exists contract_relevance;

create materialized view contract_relevance as (
    with users_per_contract as (
        select con.id from contracts con, collections col, unnest(col.nfts) as col_token_id
        inner join tokens t on t.id = col_token_id and t.deleted = false
        where t.contract_id = con.id and col.owner_user_id = t.owner_user_id
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
