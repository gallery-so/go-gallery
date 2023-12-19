package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/spf13/viper"
	"golang.org/x/term"
	"os"
)

func init() {
	viper.SetDefault("POSTGRES_USER", "gallery_migrator")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("POSTGRES_HOST", "localhost")
	viper.SetDefault("POSTGRES_PORT", "")
	viper.AutomaticEnv()
}

func main() {
	ctx := context.Background()
	db, err := connect(ctx)
	if err != nil {
		panic(err)
	}

	defer db.Close()

	numContracts := 0
	for {
		contracts, err := getContracts(ctx, db)
		if err != nil {
			panic(fmt.Errorf("error getting contracts: %w", err))
		}

		if len(contracts) == 0 {
			break
		}

		numContracts += len(contracts)
		fmt.Printf("Processing %d contracts...\n", len(contracts))

		err = processContracts(ctx, db, contracts)
		if err != nil {
			panic(fmt.Errorf("error processing contracts: %w", err))
		}
	}

	fmt.Printf("Processed %d contracts\n", numContracts)

	numCreatorAddresses := 0
	for {
		addresses, err := getCreatorAddresses(ctx, db)
		if err != nil {
			panic(fmt.Errorf("error getting addresses: %w", err))
		}

		if len(addresses) == 0 {
			break
		}

		numCreatorAddresses += len(addresses)
		fmt.Printf("Processing %d creator addresses...\n", len(addresses))

		err = processCreatorAddresses(ctx, db, addresses)
		if err != nil {
			panic(fmt.Errorf("error processing creator addresses: %w", err))
		}
	}

	fmt.Printf("Processed %d creator addresses\n", numCreatorAddresses)

	numCreatorOverrides := 0
	for {
		overrides, err := getCreatorOverrideUsers(ctx, db)
		if err != nil {
			panic(fmt.Errorf("error getting overrides: %w", err))
		}

		if len(overrides) == 0 {
			break
		}

		numCreatorOverrides += len(overrides)
		fmt.Printf("Processing %d creator overrides...\n", len(overrides))

		err = processCreatorOverrideUsers(ctx, db, overrides)
		if err != nil {
			panic(fmt.Errorf("error processing creator overrides: %w", err))
		}
	}

	fmt.Printf("Processed %d creator overrides\n", numCreatorOverrides)
}

func connect(ctx context.Context) (*sql.DB, error) {
	user := viper.GetString("POSTGRES_USER")

	fmt.Printf("Connecting to %s@%s:%s\n", user, viper.GetString("POSTGRES_HOST"), viper.GetString("POSTGRES_PORT"))
	fmt.Printf("Password for %s: ", user)

	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return nil, err
	}

	fmt.Println("\nAttempting to connect...")
	db := postgres.MustCreateClient(
		postgres.WithPassword(string(pw)),
	)

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func getContracts(ctx context.Context, db *sql.DB) ([]coredb.Contract, error) {
	query := `
	select contracts.id, contracts.deleted, contracts.version, contracts.created_at, contracts.last_updated, contracts.name, contracts.symbol, contracts.address, contracts.creator_address, contracts.chain, contracts.profile_banner_url, contracts.profile_image_url, contracts.badge_url, contracts.description, owner_address, is_provider_marked_spam, parent_id, override_creator_user_id, l1_chain
	from contracts
	         left join communities on
	            communities.community_type = 0
	        and contracts.chain::varchar(255) = communities.key1
	        and contracts.address = communities.key2
	        and not communities.deleted
	where communities.id is null
	  and not contracts.deleted
	limit 10000;
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	var contracts []coredb.Contract
	for rows.Next() {
		var contract coredb.Contract
		err := rows.Scan(
			&contract.ID,
			&contract.Deleted,
			&contract.Version,
			&contract.CreatedAt,
			&contract.LastUpdated,
			&contract.Name,
			&contract.Symbol,
			&contract.Address,
			&contract.CreatorAddress,
			&contract.Chain,
			&contract.ProfileBannerUrl,
			&contract.ProfileImageUrl,
			&contract.BadgeUrl,
			&contract.Description,
			&contract.OwnerAddress,
			&contract.IsProviderMarkedSpam,
			&contract.ParentID,
			&contract.OverrideCreatorUserID,
			&contract.L1Chain,
		)
		if err != nil {
			panic(err)
		}
		contracts = append(contracts, contract)
	}
	if err = rows.Err(); err != nil {
		panic(err)
	}

	return contracts, nil
}

type creatorAddressInfo struct {
	CommunityID    persist.DBID
	CreatorAddress persist.Address
	Chain          persist.Chain
	L1Chain        persist.L1Chain
}

func getCreatorAddresses(ctx context.Context, db *sql.DB) ([]creatorAddressInfo, error) {
	query := `
	select communities.id as community_id,
       coalesce(nullif(contracts.owner_address, ''), nullif(contracts.creator_address, '')) as creator_address,
       contracts.chain as creator_address_chain,
       contracts.l1_chain as creator_address_l1_chain
	from contracts
		join communities on communities.community_type = 0 and contracts.chain::varchar(255) = communities.key1 and contracts.address = communities.key2
		left join community_creators on community_creators.community_id = communities.id
			 and coalesce(nullif(contracts.owner_address, ''), nullif(contracts.creator_address, '')) = community_creators.creator_address
			 and contracts.chain = community_creators.creator_address_chain and contracts.l1_chain = community_creators.creator_address_l1_chain
			 and not community_creators.deleted
	where not communities.deleted and not contracts.deleted
	    and community_creators.id is null
		and coalesce(nullif(contracts.owner_address, ''), nullif(contracts.creator_address, '')) is not null
	limit 10000;
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	var results []creatorAddressInfo
	for rows.Next() {
		var info creatorAddressInfo
		err := rows.Scan(
			&info.CommunityID,
			&info.CreatorAddress,
			&info.Chain,
			&info.L1Chain,
		)
		if err != nil {
			panic(err)
		}
		results = append(results, info)
	}
	if err = rows.Err(); err != nil {
		panic(err)
	}

	return results, nil
}

type creatorOverrideUser struct {
	CommunityID   persist.DBID
	CreatorUserID persist.DBID
}

func getCreatorOverrideUsers(ctx context.Context, db *sql.DB) ([]creatorOverrideUser, error) {
	query := `
	select communities.id as community_id,
       contracts.override_creator_user_id as creator_user_id
	from contracts
		join communities on communities.community_type = 0 and contracts.chain::varchar(255) = communities.key1 and contracts.address = communities.key2
		left join community_creators on community_creators.community_id = communities.id
			 and contracts.override_creator_user_id = community_creators.creator_user_id
			 and not community_creators.deleted
	where not communities.deleted and not contracts.deleted
		and community_creators.id is null
		and contracts.override_creator_user_id is not null
	limit 10000;
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	var users []creatorOverrideUser
	for rows.Next() {
		var user creatorOverrideUser
		err := rows.Scan(
			&user.CommunityID,
			&user.CreatorUserID,
		)
		if err != nil {
			panic(err)
		}
		users = append(users, user)
	}
	if err = rows.Err(); err != nil {
		panic(err)
	}

	return users, nil
}

func processContracts(ctx context.Context, db *sql.DB, contracts []coredb.Contract) error {
	// Prepare slices for batch insert
	var id, name, description, key1, key2, profileImageURL, badgeURL, contractId []string

	for _, c := range contracts {
		id = append(id, persist.GenerateID().String())
		name = append(name, c.Name.String)
		description = append(description, c.Description.String)
		key1 = append(key1, fmt.Sprintf("%d", c.Chain))
		key2 = append(key2, c.Address.String())
		profileImageURL = append(profileImageURL, c.ProfileImageUrl.String)
		badgeURL = append(badgeURL, c.BadgeUrl.String)
		contractId = append(contractId, c.ID.String())
	}

	// Prepare the batch insert query
	batchInsertQuery := `
	with to_insert as (
	    select unnest ($1::varchar[]) as id
	         , unnest ($2::varchar[]) as name
	         , unnest ($3::varchar[]) as description
	         , unnest ($4::varchar[]) as key1
	         , unnest ($5::varchar[]) as key2
	         , unnest ($6::varchar[]) as profile_image_url
	         , unnest ($7::varchar[]) as badge_url
	         , unnest ($8::varchar[]) as contract_id
	)
	insert into communities (id, name, description, community_type, key1, key2, key3, key4, profile_image_url, badge_url, contract_id, created_at, last_updated, deleted)
	    select id, name, description, 0, key1, key2, '', '', nullif(profile_image_url, ''), nullif(badge_url, ''), contract_id, now(), now(), false from to_insert
	on conflict (community_type, key1, key2, key3, key4) where not deleted
	    do nothing;
	`

	// Execute the batch insert
	_, err := db.ExecContext(ctx, batchInsertQuery, pq.Array(id), pq.Array(name), pq.Array(description), pq.Array(key1), pq.Array(key2), pq.Array(profileImageURL), pq.Array(badgeURL), pq.Array(contractId))
	if err != nil {
		return err
	}

	return nil
}

func processCreatorAddresses(ctx context.Context, db *sql.DB, addresses []creatorAddressInfo) error {
	// Prepare slices for batch insert
	var id, communityID, address, chain, l1chain []string

	for _, i := range addresses {
		id = append(id, persist.GenerateID().String())
		communityID = append(communityID, i.CommunityID.String())
		address = append(address, i.CreatorAddress.String())
		chain = append(chain, fmt.Sprintf("%d", i.Chain))
		l1chain = append(l1chain, fmt.Sprintf("%d", i.L1Chain))
	}

	// Prepare the batch insert query
	batchInsertQuery := `
	with to_insert as (
    select unnest ($1::varchar[]) as id
         , unnest ($2::varchar[]) as community_id
         , unnest ($3::varchar[]) as address
         , unnest ($4::int[]) as chain
         , unnest ($5::int[]) as l1chain
	)
	
	insert into community_creators (id, creator_type, community_id, creator_address, creator_address_chain, creator_address_l1_chain, created_at, last_updated, deleted)
		select id, 1, community_id, address, chain, l1chain, now(), now(), false from to_insert
	on conflict (community_id, creator_type, creator_address, creator_address_l1_chain) where not deleted and creator_user_id is null
		do nothing;
	`

	// Execute the batch insert
	_, err := db.ExecContext(ctx, batchInsertQuery, pq.Array(id), pq.Array(communityID), pq.Array(address), pq.Array(chain), pq.Array(l1chain))
	if err != nil {
		return err
	}

	return nil
}

func processCreatorOverrideUsers(ctx context.Context, db *sql.DB, users []creatorOverrideUser) error {
	// Prepare slices for batch insert
	var id, communityID, creatorUserID []string

	for _, u := range users {
		id = append(id, persist.GenerateID().String())
		communityID = append(communityID, u.CommunityID.String())
		creatorUserID = append(creatorUserID, u.CreatorUserID.String())
	}

	// Prepare the batch insert query
	batchInsertQuery := `
	with to_insert as (
    select unnest ($1::varchar[]) as id
         , unnest ($2::varchar[]) as community_id
         , unnest ($3::varchar[]) as creator_user_id
	)
	
	insert into community_creators (id, creator_type, community_id, creator_user_id, created_at, last_updated, deleted)
		select id, 0, community_id, creator_user_id, now(), now(), false from to_insert
	on conflict (community_id, creator_type, creator_user_id) where not deleted and creator_user_id is not null
		do nothing;
	`

	// Execute the batch insert
	_, err := db.ExecContext(ctx, batchInsertQuery, pq.Array(id), pq.Array(communityID), pq.Array(creatorUserID))
	if err != nil {
		return err
	}

	return nil
}
