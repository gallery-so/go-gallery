create table if not exists media_validation_rules (
	id varchar(255) primary key,
	created_at timestamptz not null default now(),
	media_type varchar(32) not null,
	property varchar(32) not null,
	required boolean not null,
	unique(media_type, property) 
);
