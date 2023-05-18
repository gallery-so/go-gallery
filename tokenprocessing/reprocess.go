package tokenprocessing

const FullReprocessQuery = `left join token_medias on tokens.token_media_id = token_medias.id where tokens.deleted = false and (tokens.token_media_id is null or token_medias.active = false)`

const MissingThumbnailQuery = `left join token_medias on tokens.token_media_id = token_medias.id where tokens.deleted = false and token_medias.active = true and token_medias.media->>'media_type' = 'html' and (token_medias.media->>'thumbnail_url' is null or token_medias.media->>'thumbnail_url' = '')`
