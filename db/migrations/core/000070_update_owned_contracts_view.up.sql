begin;

drop materialized view if exists owned_contracts;
create materialized view owned_contracts as (
    with owned_contracts as (
    select
      users.id as user_id,
      users.created_at as user_created_at,
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
  displayed_tokens as (
      select
        owned_contracts.user_id,
        owned_contracts.contract_id,
        tokens.id as token_id
      from owned_contracts, galleries, collections, tokens
      where
        galleries.deleted = false
        and collections.deleted = false
        and galleries.owner_user_id = owned_contracts.user_id
        and collections.owner_user_id = owned_contracts.user_id
        and tokens.owner_user_id = owned_contracts.user_id
        and tokens.contract = owned_contracts.contract_id
        and tokens.id = any(collections.nfts)
      group by
        owned_contracts.user_id,
        owned_contracts.contract_id,
        tokens.id
  ),
  displayed_contracts as (
    select user_id, contract_id, count(token_id) as displayed_count from displayed_tokens
    group by
      user_id,
      contract_id
  )
  select
      owned_contracts.user_id,
      owned_contracts.user_created_at,
      owned_contracts.contract_id,
      owned_contracts.owned_count,
      coalesce(displayed_contracts.displayed_count, 0) as displayed_count,
      displayed_contracts.displayed_count is not null as displayed,
      now()::timestamptz as last_updated
  from owned_contracts
    left join displayed_contracts on
      displayed_contracts.user_id = owned_contracts.user_id and
      displayed_contracts.contract_id = owned_contracts.contract_id
);

create unique index owned_contracts_user_contract_idx on owned_contracts(user_id, contract_id);
create index owned_contracts_user_displayed_idx on owned_contracts(user_id, displayed);
create index owned_contracts_user_created_at_idx on owned_contracts(user_created_at);

commit;
