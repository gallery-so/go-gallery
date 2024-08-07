package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/viper"
)

var gallery = "https://api.gallery.so/glry/graphql/query/refreshToken"

func main() {
	setDefaults()

	pgClient := postgres.MustCreateClient()

	rows, err := pgClient.Query("select tokens.id from tokens join token_definitions on tokens.token_definition_id = token_definitions.id join token_medias on token_definitions.token_media_id = token_medias.id where token_medias.media->>'media_type' = 'svg' and token_medias.media->>'thumbnail_url' is null and tokens.deleted = false and tokens.displayable = true order by tokens.last_updated limit 10000;")
	if err != nil {
		panic(err)
	}

	defer rows.Close()

	p := pool.New().WithErrors().WithMaxGoroutines(100)

	for rows.Next() {

		var id persist.DBID

		err := rows.Scan(&id)
		if err != nil {
			panic(err)
		}

		fmt.Println("refreshing", id)

		gql := fmt.Sprintf(`
		mutation refreshToken {
  			refreshToken(tokenId:"%s"){
    			... on RefreshTokenPayload {
     			 	token {
       				 	media {
         					... on Media {
            					mediaURL
            					mediaType
								previewURLs {
                					raw
              					}
							}
        				}
      				}
    			}
    			... on Error {
      				message
      				__typename
    			}
  			}
		}`, id)

		jsonData := map[string]interface{}{
			"query": gql,
		}

		marshaled, err := json.Marshal(jsonData)
		if err != nil {
			panic(err)
		}
		req, err := http.NewRequest("POST", gallery, bytes.NewBuffer(marshaled))
		if err != nil {
			panic(err)
		}

		req.Header.Set("Content-Type", "application/json")

		p.Go(func() error {

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			buf := new(bytes.Buffer)
			buf.ReadFrom(resp.Body)
			fmt.Println("Returned ", buf.String(), " for ", id)
			return nil
		})

	}

	if err := p.Wait(); err != nil {
		panic(err)
	}

}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")

	viper.AutomaticEnv()

	fi := "local"
	if len(os.Args) > 1 {
		fi = os.Args[1]
	}
	envFile := util.ResolveEnvFile("tokenprocessing", fi)
	util.LoadEncryptedEnvFile(envFile)
}
