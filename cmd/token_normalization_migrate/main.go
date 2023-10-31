// Script to generate the token_definitions table from tokens and token_medias tables
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
)

const (
	poolSize  = 12  // concurrent workers to use
	batchSize = 100 // number of tokens to process per worker
)

func init() {
	rootCmd.AddCommand(saveCmd)
	rootCmd.AddCommand(migrateCmd)
	server.SetDefaults()
	viper.SetDefault("POSTGRES_USER", "gallery_migrator")
	pq = postgres.MustCreateClient()
	pq.SetMaxIdleConns(2 * poolSize)
}

func main() {
	rootCmd.Execute()
}

var rootCmd = &cobra.Command{
	Use:   "tokenmigrate",
	Short: "create token_definitions table",
}

var saveCmd = &cobra.Command{
	Use:   "stage",
	Short: "create token_chunks table for chunking tokens",
	Run: func(cmd *cobra.Command, args []string) {
		defer pq.Close()
		tx, err := pq.Begin()
		defer tx.Commit()
		check(err)
		saveStagingTable()
	},
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "create token_definitions table",
	Run: func(cmd *cobra.Command, args []string) {
		defer pq.Close()
		createTokenDefinitions(context.Background(), pq)
	},
}

var (
	pq              *sql.DB
	batchTokensStmt *sql.Stmt
	batchMediasStmt *sql.Stmt
	batchInsertStmt *sql.Stmt
)

const createStagingTable = `
drop table if exists token_chunks;

create table token_chunks(
	id serial primary key,
	chain int not null,
	contract_id varchar not null,
	token_id varchar not null,
	address varchar not null
);

insert into token_chunks(chain, contract_id, token_id, address) (
select 
	tokens.chain,
	tokens.contract,
	tokens.token_id,
	contracts.address
from tokens
join contracts on tokens.contract = contracts.id
group by 1, 2, 3, 4
order by 1, 2, 3, 4);`

const batchTokensQuery = `
with chunk as (select * from token_chunks where id >= $1 and id < $2)
select
	c.id chunk_id,
	c.chain chunk_chain,
	c.contract_id chunk_contract_id,
	c.token_id chunk_token_id,
	t.token_type token_token_type,
	t.external_url token_external_url,
	t.fallback_media token_fallback_media,
	t.deleted as token_deleted,
	c.address contract_address
from tokens t
join chunk c on (t.chain, t.contract, t.token_id) = (c.chain, c.contract_id, c.token_id)
order by t.deleted desc, t.last_updated desc;`

const batchMediasQuery = `
with chunk as (select * from token_chunks where id >= $1 and id < $2)
select
	c.id chunk_id,
	c.chain chunk_chain,
	c.contract_id chunk_contract_id,
	c.token_id chunk_token_id,
	tm.id token_media_id,
	tm.name media_name,
	tm.description media_description,
	tm.metadata media_metadata,
	tm.media media_media,
	tm.active media_active
from token_medias tm
join chunk c on (tm.chain, tm.contract_id, tm.token_id) = (c.chain, c.contract_id, c.token_id)
		and not tm.deleted
order by tm.active desc, tm.last_updated desc;`

const dropConstraints = `
alter table token_definitions set (autovacuum_enabled = false);
alter table token_definitions set unlogged;
alter table token_definitions drop constraint if exists token_definitions_pkey;
alter table token_definitions drop constraint if exists token_definitions_contract_id_fkey;
alter table token_definitions drop constraint if exists token_definitions_token_media_id_fkey;
alter table token_definitions drop constraint if exists token_definitions_contract_id_chain_contract_address_fkey;
drop index if exists token_definitions_chain_contract_id_token_idx;
drop index if exists token_definitions_chain_contract_address_token_idx;
drop index if exists token_definitions_contract_id_idx;`

const addConstraints = `
alter table token_definitions add primary key(id);
alter table token_definitions add constraint token_definitions_contract_id_fkey foreign key(contract_id) references contracts(id);
alter table token_definitions add constraint token_definitions_token_media_id_fkey foreign key(token_media_id) references token_medias(id);
alter table token_definitions add constraint token_definitions_contract_id_chain_contract_address_fkey foreign key(contract_id, chain, contract_address) references contracts(id, chain, address) on update cascade;
create unique index if not exists token_definitions_chain_contract_id_token_idx on token_definitions(chain, contract_id, token_id) where not deleted;
create unique index token_definitions_chain_contract_address_token_idx on token_definitions(chain, contract_address, token_id) where not deleted;
create index token_definitions_contract_id_idx on token_definitions(contract_id) where not deleted;
alter table token_definitions set (autovacuum_enabled = true);
alter table token_definitions set logged;
analyze token_definitions;`

const insertBatch = `
insert into token_definitions (
	id,
	name,
	description,
	external_url,
	metadata,
	fallback_media,
	contract_id,
	token_media_id,
	chain,
	contract_address,
	token_id,
	token_type
) (
	select
		id,
		nullif(name, ''),
		nullif(description, ''),
		nullif(external_url, ''),
		metadata,
		fallback_media,
		nullif(contract_id, ''),
		nullif(token_media_id, ''),
		chain,
		contract_address,
		token_id,
		token_type
	from (
		select
			unnest($1::varchar[]) id,
			unnest($2::varchar[]) name,
			unnest($3::varchar[]) description,
			unnest($4::varchar[]) external_url,
			unnest($5::jsonb[]) metadata,
			unnest($6::jsonb[]) fallback_media,
			unnest($7::varchar[]) contract_id,
			unnest($8::varchar[]) token_media_id,
			unnest($9::int[]) chain,
			unnest($10::varchar[]) contract_address,
			unnest($11::varchar[]) token_id,
			unnest($12::varchar[]) token_type
	) vals
);`

type instanceData struct {
	ChunkID            int
	Chain              persist.Chain
	ContractID         persist.NullString
	TokenTokenID       persist.NullString
	TokenName          persist.NullString
	TokenDescription   persist.NullString
	TokenTokenType     persist.TokenType
	TokenExternalURL   persist.NullString
	TokenFallbackMedia persist.FallbackMedia
	TokenMediaID       persist.NullString
	TokenDeleted       bool
	ContractAddress    persist.Address
	MediaName          persist.NullString
	MediaDescription   persist.NullString
	MediaMetadata      persist.TokenMetadata
	MediaMedia         persist.Media
	MediaActive        persist.NullBool
}

type mergedData struct {
	ID              []persist.DBID
	Name            []persist.NullString
	Description     []persist.NullString
	ExternalURL     []persist.NullString
	Metadata        []persist.TokenMetadata
	Fallback        []persist.FallbackMedia
	ContractID      []persist.NullString
	MediaID         []persist.NullString
	Chain           []persist.Chain
	ContractAddress []persist.Address
	TokenID         []persist.NullString
	TokenType       []persist.TokenType
	MediaActive     []bool
	MediaMedia      []persist.Media
}

func requireMustBeEmpty() {
	var currentCount int
	err := pq.QueryRow("select count(*) from token_definitions").Scan(&currentCount)
	check(err)
	if currentCount > 0 {
		panic(fmt.Sprintf("token_definitions table is not empty, current count: %d", currentCount))
	}
}

func analyzeTokens() {
	fmt.Print("analyzing tokens table")
	_, err := pq.Exec("analyze tokens;")
	check(err)
	fmt.Println("...done")
}

func lockTables(tx *sql.Tx) {
	_, err := tx.Exec("lock table tokens in access share mode;")
	check(err)
	_, err = tx.Exec("lock table token_medias in access share mode;")
	check(err)
}

func prepareStatements() {
	fmt.Print("preparing statements")
	var err error
	batchTokensStmt, err = pq.Prepare(batchTokensQuery)
	check(err)
	batchMediasStmt, err = pq.Prepare(batchMediasQuery)
	check(err)
	batchInsertStmt, err = pq.Prepare(insertBatch)
	check(err)
	fmt.Println("...done")
}

func dropTokenDefinitionConstraints() {
	fmt.Print("dropping constraints")
	_, err := pq.Exec(dropConstraints)
	check(err)
	fmt.Println("...done")
}

func createTokenDefinitions(ctx context.Context, pq *sql.DB) {
	globalStart := time.Now()

	defer func() {
		pq.Exec("drop table if exists token_chunks")
	}()

	tx, err := pq.BeginTx(ctx, nil)
	check(err)

	requireMustBeEmpty()
	prepareStatements()
	analyzeTokens()
	dropTokenDefinitionConstraints()
	lockTables(tx)
	totalTokens := saveStagingTable()

	wp := workerpool.New(poolSize)

	start := 0
	end := start + batchSize
	totalBatches := totalTokens / batchSize

	for chunkID := 0; start < totalTokens; chunkID++ {
		chunkID := chunkID
		s := start
		e := end
		wp.Submit(func() {
			batchStart := time.Now()

			tokenCh := make(chan bool)
			mediaCh := make(chan bool)

			var tokenRows *sql.Rows
			var tokenQueryStart time.Time
			var tokenQueryEnd time.Time
			var mediaRows *sql.Rows
			var mediaQueryStart time.Time
			var mediaQueryEnd time.Time

			go func() {
				tokenQueryStart = time.Now()
				tokenRows, err = batchTokensStmt.Query(s, e)
				check(err)
				tokenQueryEnd = time.Now()
				tokenCh <- true
			}()

			go func() {
				mediaQueryStart = time.Now()
				mediaRows, err = batchMediasStmt.Query(s, e)
				check(err)
				mediaQueryEnd = time.Now()
				mediaCh <- true
			}()

			<-tokenCh
			<-mediaCh

			idToIdx := make(map[int]int)

			m := mergedData{
				ID:              make([]persist.DBID, 0, batchSize),
				Name:            make([]persist.NullString, 0, batchSize),
				Description:     make([]persist.NullString, 0, batchSize),
				ExternalURL:     make([]persist.NullString, 0, batchSize),
				Metadata:        make([]persist.TokenMetadata, 0, batchSize),
				Fallback:        make([]persist.FallbackMedia, 0, batchSize),
				ContractID:      make([]persist.NullString, 0, batchSize),
				MediaID:         make([]persist.NullString, 0, batchSize),
				Chain:           make([]persist.Chain, 0, batchSize),
				ContractAddress: make([]persist.Address, 0, batchSize),
				TokenID:         make([]persist.NullString, 0, batchSize),
				TokenType:       make([]persist.TokenType, 0, batchSize),
				MediaActive:     make([]bool, 0, batchSize),
				MediaMedia:      make([]persist.Media, 0, batchSize),
			}

			instanceCount := 0

			for tokenRows.Next() {
				instanceCount += 1
				var r instanceData
				err := tokenRows.Scan(
					&r.ChunkID,
					&r.Chain,
					&r.ContractID,
					&r.TokenTokenID,
					&r.TokenTokenType,
					&r.TokenExternalURL,
					&r.TokenFallbackMedia,
					&r.TokenDeleted,
					&r.ContractAddress,
				)
				check(err)

				if _, ok := idToIdx[r.ChunkID]; !ok {
					idToIdx[r.ChunkID] = len(m.ID)
					m.ID = append(m.ID, persist.GenerateID())
					m.Chain = append(m.Chain, r.Chain)
					m.ContractID = append(m.ContractID, r.ContractID)
					m.TokenID = append(m.TokenID, r.TokenTokenID)
					m.TokenType = append(m.TokenType, r.TokenTokenType)
					m.ExternalURL = append(m.ExternalURL, r.TokenExternalURL)
					m.Fallback = append(m.Fallback, r.TokenFallbackMedia)
					m.ContractAddress = append(m.ContractAddress, r.ContractAddress)
					// To be filled in later from media query
					m.Name = append(m.Name, "")
					m.Description = append(m.Description, "")
					m.Metadata = append(m.Metadata, persist.TokenMetadata{})
					m.MediaID = append(m.MediaID, "")
					m.MediaActive = append(m.MediaActive, false)
					m.MediaMedia = append(m.MediaMedia, persist.Media{})
				} else if !r.TokenDeleted {
					idx := idToIdx[r.ChunkID]
					if m.TokenType[idx] == "" {
						m.TokenType[idx] = r.TokenTokenType
					}
					if m.ContractID[idx].String() == "" {
						m.ContractID[idx] = r.ContractID
					}
					if m.Fallback[idx].ImageURL.String() == "" {
						m.Metadata[idx] = r.MediaMetadata
					}
					if m.ExternalURL[idx].String() == "" {
						m.ExternalURL[idx] = r.TokenExternalURL
					}
				}
			}
			tokenRows.Close()

			for mediaRows.Next() {
				var r instanceData
				err := mediaRows.Scan(
					&r.ChunkID,
					&r.Chain,
					&r.ContractID,
					&r.TokenTokenID,
					&r.TokenMediaID,
					&r.MediaName,
					&r.MediaDescription,
					&r.MediaMetadata,
					&r.MediaMedia,
					&r.MediaActive,
				)
				check(err)
				idx, ok := idToIdx[r.ChunkID]
				if !ok {
					panic(fmt.Sprintf("no token found for media with staging_id=%d", r.ChunkID))
				}
				if m.Name[idx].String() == "" {
					m.Name[idx] = r.MediaName
				}
				if m.Description[idx].String() == "" {
					m.Description[idx] = r.MediaDescription
				}
				if len(m.Metadata[idx]) == 0 {
					m.Metadata[idx] = r.MediaMetadata
				}
				if m.MediaID[idx].String() == "" {
					m.MediaID[idx] = r.TokenMediaID
				} else if !m.MediaActive[idx] && r.MediaActive.Bool() {
					m.MediaID[idx] = r.TokenMediaID
				} else if !m.MediaMedia[idx].IsServable() && r.MediaMedia.IsServable() {
					m.MediaID[idx] = r.TokenMediaID
				} else if r.MediaMedia.MediaType.IsMorePriorityThan(m.MediaMedia[idx].MediaType) {
					m.MediaID[idx] = r.TokenMediaID
				}
			}
			mediaRows.Close()

			insertStart := time.Now()
			_, err := tx.Stmt(batchInsertStmt).Exec(
				m.ID,
				m.Name,
				m.Description,
				m.ExternalURL,
				m.Metadata,
				m.Fallback,
				m.ContractID,
				m.MediaID,
				m.Chain,
				m.ContractAddress,
				m.TokenID,
				m.TokenType,
			)
			check(err)
			insertEnd := time.Now()

			fmt.Printf("chunk(id=%d) [%d, %d) %d/%d; tokenQuery %s; mediaQuery %s; insert %s; total %s\n", chunkID, s, e, chunkID, totalBatches, tokenQueryEnd.Sub(tokenQueryStart), mediaQueryEnd.Sub(mediaQueryStart), insertEnd.Sub(insertStart), time.Since(batchStart))
		})

		start = end
		end = start + batchSize
	}

	wp.StopWait()

	fmt.Println("adding back constraints")
	now := time.Now()
	_, err = tx.Exec(addConstraints)
	check(err)

	err = tx.Commit()
	check(err)

	fmt.Printf("took %s to migrate tokens; adding constaints=%s\n", time.Since(globalStart), time.Since(now))
}

func saveStagingTable() int {
	fmt.Print("creating token_chunks table")
	r, err := pq.Exec(createStagingTable)
	check(err)
	rows, err := r.RowsAffected()
	check(err)
	_, err = pq.Exec("analyze token_chunks")
	check(err)
	fmt.Printf("...created token_chunks table with %d rows\n", rows)
	return int(rows)
}

func check(err error) {
	if err != nil {
		panic(err)
	}

}
