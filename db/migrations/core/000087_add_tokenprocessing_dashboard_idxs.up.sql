create index token_medias_processing_job_id_idx on token_medias(processing_job_id);
create index token_medias_media_media_type on token_medias((media->>'media_type'));
create index token_medias_last_updated_idx on token_medias(last_updated) where active and not deleted;
