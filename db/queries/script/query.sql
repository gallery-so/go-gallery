-- name: RandomWallets :many
select * from wallets where chain = 0 and deleted = false order by random() limit $1;
