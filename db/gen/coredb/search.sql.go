// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.26.0
// source: search.sql

package coredb

import (
	"context"
)

const searchCommunities = `-- name: SearchCommunities :many
with min_content_score as (
    select score from community_relevance where id is null
),
community_key_weights as (
    -- Using a CTE as a workaround because sqlc has trouble with this as an inline value
    -- in the ts_rank_cd statement below. We want addresses to get crazy high weighting,
    -- but ts_rank weights have to be in the [0, 1] range, so we divide the non-address weights
    -- by 1000000000 to offset the fact that we're going to multiply all addresses by 1000000000.
    select $5::float4 / 1000000000 as poap_weight,
           $6::float4 / 1000000000 as provider_weight
)
select communities.id, communities.version, communities.community_type, communities.key1, communities.key2, communities.key3, communities.key4, communities.name, communities.override_name, communities.description, communities.override_description, communities.profile_image_url, communities.override_profile_image_url, communities.badge_url, communities.override_badge_url, communities.contract_id, communities.created_at, communities.last_updated, communities.deleted, communities.website_url, communities.override_website_url, communities.mint_url, communities.override_mint_url from communities left join community_relevance on community_relevance.id = communities.id,
     to_tsquery('simple', websearch_to_tsquery('simple', $1)::text || ':*') simple_partial_query,
     websearch_to_tsquery('simple', $1) simple_full_query,
     websearch_to_tsquery('english', $1) english_full_query,
     min_content_score,
     community_key_weights,
     greatest (
        ts_rank_cd(concat('{', $2::float4, ', 1, 1, 1}')::float4[], fts_name, simple_partial_query, 1),
        ts_rank_cd(concat('{', $3::float4, ', 1, 1, 1}')::float4[], fts_description_english, english_full_query, 1),
        ts_rank_cd(concat('{', community_key_weights.poap_weight::float4, ', ', community_key_weights.provider_weight::float4, ', 1, 1}')::float4[], fts_community_key, simple_full_query, 1) * 1000000000
        ) as match_score,
     coalesce(community_relevance.score, min_content_score.score) as content_score
where (
    simple_full_query @@ fts_community_key or
    simple_partial_query @@ fts_name or
    english_full_query @@ fts_description_english
    )
    and communities.deleted = false
order by content_score * match_score desc, content_score desc, match_score desc
limit $4
`

type SearchCommunitiesParams struct {
	Query              string  `db:"query" json:"query"`
	NameWeight         float32 `db:"name_weight" json:"name_weight"`
	DescriptionWeight  float32 `db:"description_weight" json:"description_weight"`
	Limit              int32   `db:"limit" json:"limit"`
	PoapAddressWeight  float32 `db:"poap_address_weight" json:"poap_address_weight"`
	ProviderNameWeight float32 `db:"provider_name_weight" json:"provider_name_weight"`
}

func (q *Queries) SearchCommunities(ctx context.Context, arg SearchCommunitiesParams) ([]Community, error) {
	rows, err := q.db.Query(ctx, searchCommunities,
		arg.Query,
		arg.NameWeight,
		arg.DescriptionWeight,
		arg.Limit,
		arg.PoapAddressWeight,
		arg.ProviderNameWeight,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Community
	for rows.Next() {
		var i Community
		if err := rows.Scan(
			&i.ID,
			&i.Version,
			&i.CommunityType,
			&i.Key1,
			&i.Key2,
			&i.Key3,
			&i.Key4,
			&i.Name,
			&i.OverrideName,
			&i.Description,
			&i.OverrideDescription,
			&i.ProfileImageUrl,
			&i.OverrideProfileImageUrl,
			&i.BadgeUrl,
			&i.OverrideBadgeUrl,
			&i.ContractID,
			&i.CreatedAt,
			&i.LastUpdated,
			&i.Deleted,
			&i.WebsiteUrl,
			&i.OverrideWebsiteUrl,
			&i.MintUrl,
			&i.OverrideMintUrl,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const searchGalleries = `-- name: SearchGalleries :many
with min_content_score as (
    select score from gallery_relevance where id is null
)
select galleries.id, galleries.deleted, galleries.last_updated, galleries.created_at, galleries.version, galleries.owner_user_id, galleries.collections, galleries.name, galleries.description, galleries.hidden, galleries.position from galleries left join gallery_relevance on gallery_relevance.id = galleries.id,
    to_tsquery('simple', websearch_to_tsquery('simple', $1)::text || ':*') simple_partial_query,
    websearch_to_tsquery('english', $1) english_full_query,
    min_content_score,
    greatest(
        ts_rank_cd(concat('{', $2::float4, ', 1, 1, 1}')::float4[], fts_name, simple_partial_query, 1),
        ts_rank_cd(concat('{', $3::float4, ', 1, 1, 1}')::float4[], fts_description_english, english_full_query, 1)
        ) as match_score,
    coalesce(gallery_relevance.score, min_content_score.score) as content_score
where (
    simple_partial_query @@ fts_name or
    english_full_query @@ fts_description_english
    )
    and deleted = false and hidden = false
order by content_score * match_score desc, content_score desc, match_score desc
limit $4
`

type SearchGalleriesParams struct {
	Query             string  `db:"query" json:"query"`
	NameWeight        float32 `db:"name_weight" json:"name_weight"`
	DescriptionWeight float32 `db:"description_weight" json:"description_weight"`
	Limit             int32   `db:"limit" json:"limit"`
}

func (q *Queries) SearchGalleries(ctx context.Context, arg SearchGalleriesParams) ([]Gallery, error) {
	rows, err := q.db.Query(ctx, searchGalleries,
		arg.Query,
		arg.NameWeight,
		arg.DescriptionWeight,
		arg.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Gallery
	for rows.Next() {
		var i Gallery
		if err := rows.Scan(
			&i.ID,
			&i.Deleted,
			&i.LastUpdated,
			&i.CreatedAt,
			&i.Version,
			&i.OwnerUserID,
			&i.Collections,
			&i.Name,
			&i.Description,
			&i.Hidden,
			&i.Position,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const searchUsers = `-- name: SearchUsers :many
with min_content_score as (
    select score from user_relevance where id is null
)
select u.id, u.deleted, u.version, u.last_updated, u.created_at, u.username, u.username_idempotent, u.wallets, u.bio, u.traits, u.universal, u.notification_settings, u.email_unsubscriptions, u.featured_gallery, u.primary_wallet_id, u.user_experiences, u.profile_image_id, u.persona from users u left join user_relevance on u.id = user_relevance.id,
    -- Adding the search condition to the wallet join statement is a very helpful optimization, but we can't use
    -- "simple_full_query" at this point in the statement, so we're repeating the "websearch_to_tsquery..." part here
    unnest(u.wallets) as wallet_id left join wallets w on w.id = wallet_id and w.deleted = false and websearch_to_tsquery('simple', $1) @@ w.fts_address,
    to_tsquery('simple', websearch_to_tsquery('simple', $1)::text || ':*') simple_partial_query,
    websearch_to_tsquery('simple', $1) simple_full_query,
    websearch_to_tsquery('english', $1) english_full_query,
    min_content_score,
    greatest(
        ts_rank_cd(concat('{', $2::float4, ', 1, 1, 1}')::float4[], u.fts_username, simple_partial_query, 1),
        ts_rank_cd(concat('{', $3::float4, ', 1, 1, 1}')::float4[], u.fts_bio_english, english_full_query, 1),
        ts_rank_cd('{1, 1, 1, 1}', w.fts_address, simple_full_query) * 1000000000
        ) as match_score,
    coalesce(user_relevance.score, min_content_score.score) as content_score
where (
    simple_partial_query @@ u.fts_username or
    english_full_query @@ u.fts_bio_english or
    simple_full_query @@ w.fts_address
    )
    and u.universal = false and u.deleted = false
group by (u.id, content_score * match_score, content_score, match_score)
order by content_score * match_score desc, content_score desc, match_score desc, length(u.username_idempotent) asc
limit $4
`

type SearchUsersParams struct {
	Query          string  `db:"query" json:"query"`
	UsernameWeight float32 `db:"username_weight" json:"username_weight"`
	BioWeight      float32 `db:"bio_weight" json:"bio_weight"`
	Limit          int32   `db:"limit" json:"limit"`
}

func (q *Queries) SearchUsers(ctx context.Context, arg SearchUsersParams) ([]User, error) {
	rows, err := q.db.Query(ctx, searchUsers,
		arg.Query,
		arg.UsernameWeight,
		arg.BioWeight,
		arg.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []User
	for rows.Next() {
		var i User
		if err := rows.Scan(
			&i.ID,
			&i.Deleted,
			&i.Version,
			&i.LastUpdated,
			&i.CreatedAt,
			&i.Username,
			&i.UsernameIdempotent,
			&i.Wallets,
			&i.Bio,
			&i.Traits,
			&i.Universal,
			&i.NotificationSettings,
			&i.EmailUnsubscriptions,
			&i.FeaturedGallery,
			&i.PrimaryWalletID,
			&i.UserExperiences,
			&i.ProfileImageID,
			&i.Persona,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
