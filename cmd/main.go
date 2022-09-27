package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func main() {
	finalResult := []map[string]interface{}{}
	offset := 0
	for {
		resp, err := http.Get(fmt.Sprintf("https://api.tzkt.io/v1/tokens/balances?token.standard=fa2&account=tz1epSqLxcYbTGAAgwKhmBdVhTgAWrtUkz8G&limit=1000&sort.asc=id&offset=%d", offset))
		if err != nil {
			panic(err)
		}
		result := []map[string]interface{}{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			panic(err)
		}
		finalResult = append(finalResult, result...)
		if len(result) < 1000 {
			break
		}
		offset += 1000
	}
	fmt.Println(len(finalResult))
}
