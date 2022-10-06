-- name: FirstContract :one
-- sqlc needs at least one query in order to generate the models.
SELECT * FROM contracts LIMIT 1;
