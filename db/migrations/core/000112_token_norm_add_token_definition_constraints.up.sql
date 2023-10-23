/* Migrate to rebuild the tokens table with the token_definition_id column */
begin;

-- Temporarily increase work_mem to speed up the migration so prevent data spilling to disk
set local work_mem = '5500 MB';

-- Only allow reads to the table
lock table tokens in access exclusive mode;

-- Create a copy of the tokens table with the new column
create table tokens_with_token_definition_fk as
select
	t.id,
	t.deleted,
	t.version,
	t.created_at,
	t.last_updated,
	t.name,
	t.description,
	t.collectors_note,
	t.token_type,
	t.token_id,
	t.quantity,
	t.ownership_history,
	t.external_url,
	t.block_number,
	t.owner_user_id,
	t.owned_by_wallets,
	t.chain,
	t.contract,
	t.is_user_marked_spam,
	t.is_provider_marked_spam,
	t.last_synced,
	t.token_uri,
	t.fallback_media,
	t.token_media_id,
	t.is_creator_token,
	td.id token_definition_id
from tokens t
join token_definitions td on (t.chain, t.contract, t.token_id) = (td.chain, td.contract_id, td.token_id) and not td.deleted;

-- Add back the generated columns
alter table tokens_with_token_definition_fk add column is_holder_token boolean not null generated always as (cardinality(owned_by_wallets) > 0) stored;
alter table tokens_with_token_definition_fk add column displayable boolean not null generated always as (cardinality(owned_by_wallets) > 0 or is_creator_token) stored;

-- Add back constraints and indices
alter table tokens_with_token_definition_fk add primary key(id);
alter table tokens_with_token_definition_fk add constraint contracts_contract_id_fkey foreign key(contract) references contracts(id) on delete cascade on update cascade;
alter table tokens_with_token_definition_fk alter column token_definition_id set not null;
alter table tokens_with_token_definition_fk alter column contract set not null;
alter table tokens_with_token_definition_fk add constraint token_definitions_token_definition_id_fkey foreign key(token_definition_id) references token_definitions(id) on update cascade;
create unique index on tokens_with_token_definition_fk(owner_user_id, token_definition_id) where not deleted;
create index on tokens_with_token_definition_fk(owner_user_id, is_creator_token) where deleted = false;
create index on tokens_with_token_definition_fk(owner_user_id, is_holder_token) where deleted = false;
create index on tokens_with_token_definition_fk(owner_user_id, displayable) where deleted = false;
create index on tokens_with_token_definition_fk(owner_user_id, contract_id) where deleted = false;
create index on tokens_with_token_definition_fk using gin (owned_by_wallets);
create index on tokens_with_token_definition_fk(last_updated) where deleted = false;
create index on tokens_with_token_definition_fk(contract_id, token_definition_id) where deleted = false;

-- Rename the table, create a backup of the old table
alter table tokens rename to tokens_backup;
alter table tokens_with_token_definition_fk rename to tokens;

-- Rename columns while were at it
alter table tokens rename column contract to contract_id;

-- Keep migrated columns around for a bit
alter table tokens rename column chain to chain__deprecated;
alter table tokens rename column token_id to token_id__deprecated;
alter table tokens rename column name to name__deprecated;
alter table tokens rename column description to description__deprecated;
alter table tokens rename column token_type to token_type__deprecated;
alter table tokens rename column ownership_history to ownership_history__deprecated;
alter table tokens rename column external_url to external_url__deprecated;
alter table tokens rename column is_provider_marked_spam to is_provider_marked_spam__deprecated;
alter table tokens rename column token_uri to token_uri__deprecated;
alter table tokens rename column fallback_media to fallback_media__deprecated;
alter table tokens rename column token_media_id to token_media_id__deprecated;

end;
