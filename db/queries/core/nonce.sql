-- name: InsertNonce :one
insert into nonces (id, value) values (@id, @value)
    on conflict (value)
        do nothing
    returning *;

-- name: ConsumeNonce :one
update nonces
    set
        consumed = true
    where
        value = @value
        and not consumed
        and nonces.created_at > (now() - interval '1 hour')
    returning *;