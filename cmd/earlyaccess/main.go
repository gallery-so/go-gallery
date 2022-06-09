package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/lib/pq"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Early access address adder!
// Build from go-gallery root like this: go build -o ./bin/earlyaccess ./cmd/earlyaccess/main.go
// And run like this: ./bin/earlyaccess "postgres://postgres:<dev db password here>@34.102.59.201:5432/postgres" "../snapshots/snapshot.json"
func main() {
	args := os.Args

	if len(args) != 3 || args[1] == "help" {
		fmt.Print("\nThis program adds address entries from a JSON file to the early_access database table.\n")
		fmt.Print("It is safe to attempt re-adding addresses that have already been added to the database; they will be ignored.\n\n")
		fmt.Printf("syntax: %s <sqlConnectionString> <pathToJson>\n", args[0])
		fmt.Printf("example: %s \"postgres://postgres:<dev db password here>@34.102.59.201:5432/postgres\" \"../snapshots/snapshot.json\"\n", args[0])
		os.Exit(0)
	}

	connectionStr := args[1]
	path := args[2]

	file, err := os.Open(path)
	exitOnError("could not read file", err)

	reader := bufio.NewReader(file)
	addresses, err := jsonToAddresses(reader)
	exitOnError("parsing json failed", err)

	err = file.Close()
	exitOnError("error closing file", err)

	numAddresses := len(addresses)
	fmt.Printf("read %d addresses from %s\n", numAddresses, filepath.Base(path))

	addresses = removeDuplicateAddresses(addresses)
	if len(addresses) != numAddresses {
		fmt.Printf("removed %d duplicate addresses\n", numAddresses-len(addresses))
		numAddresses = len(addresses)
	}

	err = insertAddresses(context.Background(), connectionStr, addresses)
	exitOnError("failed to insert addresses into database", err)
}

func exitOnError(message string, err error) {
	if err == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
	os.Exit(1)
}

func jsonToAddresses(input io.Reader) ([]string, error) {
	var addresses []string

	err := json.NewDecoder(input).Decode(&addresses)
	if err != nil {
		return nil, err
	}

	for i, address := range addresses {
		addresses[i] = strings.ToLower(address)
	}

	return addresses, nil
}

func removeDuplicateAddresses(addresses []string) []string {
	seen := make(map[string]bool)
	filtered := addresses[:0]

	for _, address := range addresses {
		if _, ok := seen[address]; ok {
			continue
		}

		filtered = append(filtered, address)
		seen[address] = true
	}

	return filtered
}

func insertAddresses(ctx context.Context, connectionStr string, addresses []string) error {
	conn, err := pgx.Connect(ctx, connectionStr)
	exitOnError("unable to connect to database", err)

	defer conn.Close(ctx)

	const countQuery = "SELECT COUNT(*) FROM early_access;"
	var startingCount int
	err = conn.QueryRow(ctx, countQuery).Scan(&startingCount)
	exitOnError("error counting number of addresses in early_access table", err)

	fmt.Printf("inserting %d rows into early_access table...\n", len(addresses))

	const insertQuery = "INSERT INTO early_access (address) SELECT unnest($1::TEXT[]) ON CONFLICT DO NOTHING;"
	_, err = conn.Exec(ctx, insertQuery, pq.Array(addresses))
	exitOnError("executing query failed", err)

	fmt.Printf("successfully inserted addresses into early_access table\n")

	var endingCount int
	err = conn.QueryRow(ctx, countQuery).Scan(&endingCount)
	exitOnError("error counting number of addresses in early_access table", err)

	fmt.Printf("added %d unique new rows\nearly_access table now contains: %d rows\n", endingCount-startingCount, endingCount)

	return nil
}
