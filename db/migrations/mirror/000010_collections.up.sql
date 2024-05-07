drop table if exists ethereum.collections;
drop table if exists base.collections;
drop table if exists zora.collections;

alter table collections add column name text;
alter table collections add column description text;
alter table collections add column image_url text;
alter table collections add column banner_image_url text;
alter table collections add column category text;
alter table collections add column is_nsfw bool;
alter table collections add column external_url text;
alter table collections add column twitter_username text;
alter table collections add column discord_url text;
alter table collections add column instagram_url text;
alter table collections add column medium_username text;
alter table collections add column telegram_url text;
alter table collections add column marketplace_pages jsonb;
alter table collections add column metaplex_mint text;
alter table collections add column metaplex_candy_machine text;
alter table collections add column metaplex_first_verified_creator text;
alter table collections add column spam_score int;
alter table collections add column chains text[];
alter table collections add column top_contracts text[];
alter table collections add column collection_royalties jsonb;

