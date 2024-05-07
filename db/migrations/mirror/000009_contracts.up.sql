alter table ethereum.contracts add column type text;
alter table ethereum.contracts add column name text;
alter table ethereum.contracts add column symbol text;
alter table ethereum.contracts add column deployed_by text;
alter table ethereum.contracts add column deployed_via_contract text;
alter table ethereum.contracts add column owned_by text;
alter table ethereum.contracts add column has_multiple_collections boolean;

alter table base.contracts add column type text;
alter table base.contracts add column name text;
alter table base.contracts add column symbol text;
alter table base.contracts add column deployed_by text;
alter table base.contracts add column deployed_via_contract text;
alter table base.contracts add column owned_by text;
alter table base.contracts add column has_multiple_collections boolean;

alter table zora.contracts add column type text;
alter table zora.contracts add column name text;
alter table zora.contracts add column symbol text;
alter table zora.contracts add column deployed_by text;
alter table zora.contracts add column deployed_via_contract text;
alter table zora.contracts add column owned_by text;
alter table zora.contracts add column has_multiple_collections boolean;