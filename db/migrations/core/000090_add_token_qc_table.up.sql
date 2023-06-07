create table if not exists token_processing_qc_dashboard (
	token_media_id character varying(255) primary key references token_medias(id),
	media_type character varying(255) not null,
	last_updated timestamp with time zone not null,
	is_valid boolean not null,
	reason character varying(255),
	ran_on timestamp with time zone not null default now()
);
create index token_processing_qc_dashboard_last_updated_ix on token_processing_qc_dashboard(last_updated);
create index token_processing_qc_dashboard_media_type on token_processing_qc_dashboard(media_type);
create index tokens_token_media_id on tokens(token_media_id) where not deleted;
