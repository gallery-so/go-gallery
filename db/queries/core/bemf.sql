-- name: GetOwnedContractsByUser :many
select t.username_idempotent username, array_agg(t.address)::varchar[] as addresses
from (
	select users.username_idempotent, contracts.address
	from tokens
	join contracts on tokens.contract = contracts.id
	join users on tokens.owner_user_id = users.id
	where not tokens.deleted
	  and tokens.displayable
	  and not contracts.deleted
	  and not users.deleted
	  and not users.universal
	group by users.username_idempotent, contracts.address
) t
group by t.username_idempotent;

-- name: GetDisplayedContractsByUser :many
select t.username_idempotent username, array_agg(t.address)::varchar[] as addresses
from (
	select users.username_idempotent, contracts.address
	from tokens
	join contracts on tokens.contract = contracts.id
	join users on tokens.owner_user_id = users.id
	where not tokens.deleted
	  and tokens.displayable
	  and not contracts.deleted
	  and not users.deleted
	  and not users.universal
	group by users.username_idempotent, contracts.address
) t
group by t.username_idempotent;

-- name: GetPostedContractsByUser :many
select t.username_idempotent username, array_agg(t.address)::varchar[] as addresses
from (
	select users.username_idempotent, contracts.address
	from posts
	join users on posts.actor_id = users.id
	join contracts on contracts.id = any(posts.contract_ids)
	where not users.deleted
		and not users.universal
		and not posts.deleted
	group by users.username_idempotent, contracts.address
) t
group by t.username_idempotent;

-- name: GetViewedContractsByUser :many
select t.username_idempotent username, array_agg(t.address)::varchar[] as addresses
from (
	select users.username_idempotent, contracts.address
	from events
	join tokens on tokens.id = events.token_id
	join users on events.actor_id = users.id
	join contracts on tokens.contract = contracts.id
	where not events.deleted
		and action = 'ViewedToken'
		and not tokens.deleted
		and not users.deleted
		and not contracts.deleted
	group by users.username_idempotent, contracts.address
) t
group by t.username_idempotent;
