-----------------------------------------------------------------------------------
-- sqlc customizations
-----------------------------------------------------------------------------------
-- Anything added here will be processed by sqlc after it finishes reading our
-- regular core migrations. Schema modifications in this file will NOT be run
-- against our database, but can be used to alter sqlc's view of our definitions.
-- For example, if sqlc should ignore a table, that can be achieved by adding
-- a "drop table" statement here.
-----------------------------------------------------------------------------------
-- NOTE: THIS TECHNIQUE SHOULD BE USED SPARINGLY.
-- We typically _do_ want sqlc to have an accurate view of our schema, and
-- altering sqlc's view of our schema should be a last resort!
-----------------------------------------------------------------------------------

-- Hide tsvector search columns from sqlc. We don't actually want to select them when
-- we select all columns in a table, and pgx can't handle them, so "select *" queries
-- will fail if we don't hide these columns.
alter table users drop column if exists fts_username;
alter table users drop column if exists fts_bio_english;

alter table wallets drop column if exists fts_address;

alter table contracts drop column if exists fts_name;
alter table contracts drop column if exists fts_address;
alter table contracts drop column if exists fts_description_english;

alter table galleries drop column if exists fts_name;
alter table galleries drop column if exists fts_description_english;


