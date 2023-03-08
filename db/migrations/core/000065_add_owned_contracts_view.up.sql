begin;

drop materialized view if exists owned_contracts;
create materialized view owned_contracts as (
  with displayed_contracts as (
    select
      tokens.owner_user_id as user_id,
      contracts.id as contract_id
    from contracts, tokens, collections
    where
      contracts.id = tokens.contract
      and collections.owner_user_id = tokens.owner_user_id
      and collections.deleted = false
      and tokens.id = any (collections.nfts)
      group by 1, 2
      having count(collections.id) > 0
  )
  select
    users.id as user_id,
    contracts.id as contract_id,
    count(tokens.id) as owned_count,
    count(displayed_contracts.user_id) > 0 as displayed,
    now()::timestamp as last_updated
  from users
  join tokens on tokens.deleted = false and users.id = tokens.owner_user_id and coalesce(tokens.is_user_marked_spam, false) = false
  join contracts on contracts.deleted = false and tokens.contract = contracts.id
  left join displayed_contracts on users.id = displayed_contracts.user_id and contracts.id = displayed_contracts.contract_id
  where users.deleted = false
  group by 1, 2
);

create unique index owned_contracts_user_contract_idx on owned_contracts(user_id, contract_id);
create index owned_contract_user_displayed_idx on owned_contracts(user_id, displayed);
select cron.schedule('0 */3 * * *', 'refresh materialized view concurrently owned_contracts with data');

commit;
