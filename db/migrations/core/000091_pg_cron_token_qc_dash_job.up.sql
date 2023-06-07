select cron.schedule(
  'tokenprocessing-qc-dashboard-update',
  '0 * * * *',
  $$with report_start as (
	  select coalesce(max(ran_on), now() - interval '7 day') last_ran from token_processing_qc_dashboard
  ),
  new_media as (
    with active_media as (
      select 
        token_medias.id
        , token_medias.last_updated
        , token_medias.media->>'media_type' media_type
        , token_processing_jobs.id job_id
        , token_processing_jobs.token_properties
        , token_processing_jobs.pipeline_metadata
      from token_medias
      join token_processing_jobs on token_medias.processing_job_id = token_processing_jobs.id 
      join contracts on contracts.id = token_medias.contract_id
      where 
        token_medias.active
        and not token_medias.deleted
        and not contracts.is_provider_marked_spam
        and token_medias.last_updated between (select last_ran from report_start) and now()
    ),
    -- media types that are unmapped
    media_no_validation_rules as (
      select m.id, m.media_type, m.last_updated, false is_valid, 'no validation rules' reason
      from active_media m
      where not exists (select 1 from media_validation_rules where m.media_type = media_validation_rules.media_type)
    ),
    -- media that is missing required properties
    media_missing_property as (
      select id, media_type, last_updated, false is_valid, string_agg('missing prop: ' || property, ', ') reason
      from active_media, jsonb_each(token_properties) props(property, has_property)
      where exists (
        select 1
        from media_validation_rules media_rules
        where active_media.media_type = media_rules.media_type 
          and media_rules.property = props.property
          and media_rules.required
          and not props.has_property::boolean
      )
      -- exclude tokens that aren't retrievable
      and active_media.pipeline_metadata->>'media_urls_retrieval' = 'success' and (
  		active_media.pipeline_metadata->>'image_reader_retrieval' = 'success'
  		or active_media.pipeline_metadata->>'animation_reader_retrieval' = 'success'
  		or active_media.pipeline_metadata->>'alternate_reader_retrieval' = 'success'
  	)
      group by id, media_type, last_updated, is_valid
    ),
    -- media that is valid
    media_valid as (
      select id, media_type, last_updated, true is_valid, null reason
      from active_media
      where not exists (select 1 from media_missing_property where media_missing_property.id = active_media.id)
        and not exists (select 1 from media_no_validation_rules where media_no_validation_rules.id = active_media.id)
    )
    select * from media_valid
    union
    select * from media_missing_property
    union 
    select * from media_no_validation_rules
  ),
  insert_media as (
  	insert into token_processing_qc_dashboard (select * from new_media)
  	on conflict (token_media_id) do update set media_type = excluded.media_type, last_updated = excluded.last_updated, is_valid = excluded.is_valid, reason = excluded.reason, ran_on = now()
  	returning ran_on
  )
  -- keep two months of data around
  delete from token_processing_qc_dashboard where last_updated <= (select ran_on - interval '60 day' from insert_media limit 1)$$);
