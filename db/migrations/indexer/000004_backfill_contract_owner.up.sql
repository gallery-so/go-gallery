create table if not exists reprocess_jobs (
    id int primary key,
    start_id varchar(255) not null,
    end_id varchar(255) not null
);

alter table contracts add column if not exists owner_method int;