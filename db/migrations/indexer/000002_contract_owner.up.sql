alter table contracts add column owner_address character varying(255);
alter table contracts drop column creator_address;