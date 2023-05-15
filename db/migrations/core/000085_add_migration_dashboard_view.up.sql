drop materialized view if exists migration_validation;
create materialized view migration_validation as (
	select
		t.id 
		, m.id media_id
		, m.processing_job_id
		, t.chain
		, t.contract
		, t.token_id
		, case when t.media->>'media_type' = '' then 'empty' else t.media->>'media_type' end media_type
		, case when m.media->>'media_type' = '' then 'empty'when m.media->>'media_type' is null then 'unmapped' else m.media->>'media_type' end remapped_to
		, t.media old_media
		, m.media new_media
		, case 
			when (t.media is not null and m.media is null)
			then 'no mapped media'
			when (t.media->>'media_type' != m.media->>'media_type')
			then 
				case when (t.media->>'media_type' not in ('', 'syncing', 'invalid', 'unknown') and m.media->>'media_type' in ('', 'invalid', 'unknown'))
				then 'found worse media'
				when (t.media->>'media_type' in ('', 'syncing', 'invalid', 'unknown') and m.media->>'media_type' not in ('', 'syncing', 'invalid', 'unknown'))
				then 'found better media'
				when (t.media->>'media_type' in ('', 'syncing') and m.media->>'media_type' in ('', 'invalid', 'unknown'))
				then 'previously syncing now unknown'
				else 'media type changed' end
			else 'no change'
		end media_type_validation
		, case 
			when (t.media is not null and m.media is null)
			then 'no mapped media'
			when ((t.media->'dimensions'->>'width')::int != 0 and (m.media->'dimensions'->>'width')::int = 0)
			then 'now missing dimensions'
			when ((t.media->'dimensions'->>'width')::int = 0 and (m.media->'dimensions'->>'width')::int != 0)
			then 'now has dimensions'
			else 'no change' end dimensions_validation
		, case
			when (t.media is not null and m.media is null)
			then 'no mapped media'
			when t.media->>'media_url' is not null and m.media->>'media_url' is null
			then 'now missing media_url'
			when t.media->>'media_url' is null and m.media->>'media_url' is not null
			then 'now has media_url'
			else 'no change' end media_url_validation
		, case
			when (t.media is not null and m.media is null)
			then 'no mapped media'
			when t.media->>'thumbnail_url' is not null and m.media->>'thumbnail_url' is null
			then 'now missing thumbnail_url'
			when t.media->>'thumbnail_url' is null and m.media->>'thumbnail_url' is not null
			then 'now has thumbnail_url'
			else 'no change' end thumbnail_url_validation
		, case
			when (t.media is not null and m.media is null)
			then 'no mapped media'
			when t.media->>'live_preview_url' is not null and m.media->>'live_preview_url' is null
			then 'now missing live_preview_url'
			when t.media->>'live_preview_url' is null and m.media->>'live_preview_url' is not null
			then 'now has live_preview_url'
			else 'no change' end live_preview_url_validation
		, now() as last_refreshed
	from tokens t
	join contracts c on t.contract = c.id
	left join token_medias m on t.chain = m.chain and t.contract = m.contract_id and t.token_id = m.token_id and m.active and not m.deleted
	where not t.deleted and not coalesce(t.is_provider_marked_spam, false) and not coalesce(t.is_user_marked_spam, false) and not c.is_provider_marked_spam
);
drop index if exists migration_validation_media_type_media_type_validation;
create index migration_validation_media_type_media_type_validation on migration_validation(media_type, media_type_validation);
drop index if exists migration_validation_media_type_dimensions_validation;
create index migration_validation_media_type_dimensions_validation on migration_validation(media_type, dimensions_validation);
drop index if exists migration_validation_media_type_media_media_url_validation;
create index migration_validation_media_type_media_url_validation on migration_validation(media_type, media_url_validation);
drop index if exists migration_validation_media_type_media_thumbnail_url_validation;
create index migration_validation_media_type_thumbnail_url_validation on migration_validation(media_type, thumbnail_url_validation);
drop index if exists migration_validation_media_type_live_preview_url_validation;
create index migration_validation_media_type_live_preview_url_validation on migration_validation(media_type, live_preview_url_validation);
drop index if exists migration_validation_media_type;
create index migration_validation_media_type on migration_validation(media_type);

select cron.schedule('tokenprocessing-migration-dashboard',  '0 * * * *', 'refresh materialized view migration_validation');
