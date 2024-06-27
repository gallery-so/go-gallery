-- Base
alter table base.tokens add column if not exists extra_metadata_jsonb jsonb default '{}'::jsonb;
alter table base.tokens add column if not exists last_metadata_conversion timestamptz default make_date(2000, 1, 1);

-- Create an index for Moshi border IDs. Only needs to be done on Base.
create index if not exists base_tokens_border_id_idx on base.tokens ((extra_metadata_jsonb->'properties'->>'borderId'));

-- Make an index so we can find rows that still need to be converted
create index if not exists base_temp_metadata_index on base.tokens(last_metadata_conversion) where last_metadata_conversion < last_updated;

-- Ethereum
alter table ethereum.tokens add column if not exists extra_metadata_jsonb jsonb default '{}'::jsonb;
alter table ethereum.tokens add column if not exists last_metadata_conversion timestamptz default make_date(2000, 1, 1);
create index if not exists ethereum_temp_metadata_index on ethereum.tokens(last_metadata_conversion) where last_metadata_conversion < last_updated;

-- Zora
alter table zora.tokens add column if not exists extra_metadata_jsonb jsonb default '{}'::jsonb;
alter table zora.tokens add column if not exists last_metadata_conversion timestamptz default make_date(2000, 1, 1);
create index if not exists zora_temp_metadata_index on zora.tokens(last_metadata_conversion) where last_metadata_conversion < last_updated;

-- Base Sepolia
alter table base_sepolia.tokens add column if not exists extra_metadata_jsonb jsonb default '{}'::jsonb;
alter table base_sepolia.tokens add column if not exists last_metadata_conversion timestamptz default make_date(2000, 1, 1);
create index if not exists base_sepolia_temp_metadata_index on base_sepolia.tokens(last_metadata_conversion) where last_metadata_conversion < last_updated;