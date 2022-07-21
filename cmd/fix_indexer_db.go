package main

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/gammazero/workerpool"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/spf13/viper"
)

type tid struct {
	tokenID         string
	contractAddress string
}

func main() {
	setDefaults()

	p := postgres.NewPgxClient()

	fmt.Println("Starting...")

	for {
		done := false

		func() {

			fmt.Println("Starting batch...")
			receivedTokenIdentifiers := make(chan tid)

			wg := &sync.WaitGroup{}
			wg.Add(1)

			go func() {
				defer wg.Done()
				wp := workerpool.New(50)
				for t := range receivedTokenIdentifiers {
					token := t
					wp.Submit(func() {
						fmt.Printf("%s %s\n", token.tokenID, token.contractAddress)

						findAndMergeInaccurateDupes(p, token.tokenID, token.contractAddress)

						fmt.Printf("done %s %s\n", token.tokenID, token.contractAddress)
					})
				}
				wp.StopWait()
			}()

			i := 0
			var tokenID, contractAddress string
			_, err := p.QueryFunc(context.Background(), `SELECT token_id, contract_address 
			FROM tokens 
			WHERE TOKEN_TYPE = 'ERC-721'
			GROUP BY contract_address, token_id 
			HAVING COUNT(*) > 1 
			LIMIT 200000;`, []interface{}{}, []interface{}{&tokenID, &contractAddress}, func(qfr pgx.QueryFuncRow) error {
				i++
				receivedTokenIdentifiers <- tid{tokenID, contractAddress}
				return nil
			})
			if err != nil {
				panic(err)
			}

			if i == 0 {
				done = true
			}

			close(receivedTokenIdentifiers)

			wg.Wait()
		}()
		if done {
			break
		}
	}

	fmt.Println("done")
}

type mergeData struct {
	dbid               string
	ownershipHistories []persist.AddressAtBlock
	block              uint64
}

func findAndMergeInaccurateDupes(p *pgxpool.Pool, tokenID, contractAddress string) {
	rows, err := p.Query(context.Background(), `SELECT ID,OWNERSHIP_HISTORY,BLOCK_NUMBER FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT_ADDRESS = $2;`, tokenID, contractAddress)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	var data []mergeData

	for rows.Next() {
		var id string
		var ownershipHistory []persist.AddressAtBlock
		var blockNumber uint64
		err := rows.Scan(&id, pq.Array(&ownershipHistory), &blockNumber)
		if err != nil {
			panic(err)
		}
		data = append(data, mergeData{
			dbid:               id,
			ownershipHistories: ownershipHistory,
			block:              blockNumber,
		})
	}

	sort.SliceStable(data, func(i, j int) bool {
		return data[i].block > data[j].block
	})

	theRealOneOG := data[0]
	for _, d := range data[1:] {
		theRealOneOG.ownershipHistories = append(theRealOneOG.ownershipHistories, d.ownershipHistories...)
	}

	// update ownership history for the real one
	_, err = p.Exec(context.Background(), `UPDATE tokens SET OWNERSHIP_HISTORY = $1 WHERE ID = $2;`, theRealOneOG.ownershipHistories, theRealOneOG.dbid)
	if err != nil {
		panic(err)
	}

	// delete the bad ones
	for _, d := range data[1:] {
		_, err = p.Exec(context.Background(), `DELETE FROM tokens WHERE ID = $1`, d.dbid)
		if err != nil {
			panic(err)
		}
	}

}
func setDefaults() {
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("INDEXER_HOST", "http://localhost:4000")

	viper.AutomaticEnv()
}
