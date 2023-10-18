-- name: GetTokenBackfillBatch :many
select sqlc.embed(tokens), sqlc.embed(token_medias)
from tokens
left join token_medias on tokens.token_media_id__deprecated = token_medias.id
where @start_id < tokens.id and tokens.id <= @end_id;

-- name: GetTokenBackfillExcess :many
select id
from tokens
where id >= @excess_id;
