-- token_medias_active is a view that contains all active token medias
create or replace view token_medias_active as (
	select
		token_medias.id,
		token_medias.last_updated,
		token_medias.media ->> 'media_type' media_type,
		token_processing_jobs.id job_id,
		token_processing_jobs.token_properties,
		token_processing_jobs.pipeline_metadata
	from
		token_medias
		join token_processing_jobs on token_medias.processing_job_id = token_processing_jobs.id
	where
		token_medias.active
		and not token_medias.deleted
);

-- token_medias_no_validation_rules is a view that contains all active token medias that are missing validation rules
create or replace view token_medias_no_validation_rules as (
	select
		m.id,
		m.media_type,
		m.last_updated,
		false is_valid,
		'no validation rules' reason
	from
		token_medias_active m
	where
		not exists (
			select
				1
			from
				media_validation_rules
			where
				m.media_type = media_validation_rules.media_type)
);


-- token_medias_missing_properties is a view that contains all active, invalid token medias that are missing required token properties
create or replace view token_medias_missing_properties as (
	select
		id,
		media_type,
		last_updated,
		false is_valid,
		string_agg('missing prop: ' || property, ', ') reason
	from
		token_medias_active,
		jsonb_each(token_properties) props (property, has_property)
	where
		exists (
			select
				1
			from
				media_validation_rules media_rules
			where
				token_medias_active.media_type = media_rules.media_type
				and media_rules.property = props.property
				and media_rules.required
				and not props.has_property::boolean)
			-- only include tokens that were retrievable
			and token_medias_active.pipeline_metadata ->> 'media_urls_retrieval' = 'success'
			and (token_medias_active.pipeline_metadata ->> 'image_reader_retrieval' = 'success'
				or token_medias_active.pipeline_metadata ->> 'animation_reader_retrieval' = 'success'
				or token_medias_active.pipeline_metadata ->> 'alternate_reader_retrieval' = 'success')
		group by
			id,
			media_type,
			last_updated,
			is_valid
);
