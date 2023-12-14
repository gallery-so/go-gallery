alter table mentions drop column contract_id;
alter table mentions add column community_id varchar(255) references communities(id);

alter table notifications rename column contract_id to community_id;

alter table events rename column contract_id to community_id;