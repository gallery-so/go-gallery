alter table contracts drop column owner_address;
alter table contracts add column creator_address character varying(255);