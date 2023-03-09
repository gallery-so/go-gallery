begin;

drop materialized view if exists owned_contracts;
create materialized view owned_contracts as (
    with owned_contracts as (
    select
      users.id as user_id,
      contracts.id as contract_id,
      count(tokens.id) as owned_count
    from users
    join tokens on
      tokens.deleted = false
      and users.id = tokens.owner_user_id
      and coalesce(tokens.is_user_marked_spam, false) = false
    join contracts on
      contracts.deleted = false
      and tokens.contract = contracts.id
    where
      users.deleted = false
      and users.universal = false
    group by
      users.id,
      contracts.id
  ),
  displayed_contracts as (
    select
      owned_contracts.user_id,
      owned_contracts.contract_id
    from owned_contracts, galleries, collections, tokens
    where
      galleries.deleted = false
      and collections.deleted = false
      and galleries.owner_user_id = owned_contracts.user_id
      and collections.owner_user_id = owned_contracts.user_id
      and tokens.owner_user_id = owned_contracts.user_id
      and tokens.contract = owned_contracts.contract_id
      and tokens.id = any(collections.nfts)
  )
  select
      owned_contracts.user_id,
      owned_contracts.contract_id,
      owned_contracts.owned_count,
      exists(
        select 1
        from displayed_contracts
        where displayed_contracts.user_id = owned_contracts.user_id and displayed_contracts.contract_id = owned_contracts.contract_id
        limit 1
      ) as displayed,
      now()::timestamp as last_updated
  from owned_contracts
);

create unique index owned_contracts_user_contract_idx on owned_contracts(user_id, contract_id);
create index owned_contract_user_displayed_idx on owned_contracts(user_id, displayed);
select cron.schedule('0 */3 * * *', 'refresh materialized view concurrently owned_contracts with data');

commit;
