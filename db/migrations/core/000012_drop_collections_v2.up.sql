DROP TABLE IF EXISTS collections_v2;

ALTER TABLE collections ALTER COLUMN layout TYPE jsonb USING layout::jsonb;