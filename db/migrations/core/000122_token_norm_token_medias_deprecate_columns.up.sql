alter table token_medias rename column name to name__deprecated;
alter table token_medias rename column description to description__deprecated;
alter table token_medias rename column metadata to metadata__deprecated;
alter table token_medias rename column contract_id to contract_id__deprecated;
alter table token_medias rename column token_id to token_id__deprecated;
alter table token_medias rename column chain to chain__deprecated;
alter table token_medias alter column name__deprecated drop not null;
alter table token_medias alter column description__deprecated drop not null;
alter table token_medias alter column metadata__deprecated drop not null;
alter table token_medias alter column contract_id__deprecated drop not null;
alter table token_medias alter column token_id__deprecated drop not null;
alter table token_medias alter column chain__deprecated drop not null;