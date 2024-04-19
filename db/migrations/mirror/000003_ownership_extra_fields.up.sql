alter table ethereum.owners add column if not exists created_at timestamptz not null default now();
alter table ethereum.owners add column if not exists last_updated timestamptz not null default now();
alter table ethereum.owners add column if not exists kafka_offset bigint;
alter table ethereum.owners add column if not exists kafka_partition int;
alter table ethereum.owners add column if not exists kafka_timestamp timestamptz;