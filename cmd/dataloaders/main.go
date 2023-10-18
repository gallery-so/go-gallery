package main

import "github.com/mikeydub/go-gallery/cmd/dataloaders/generator"

func main() {
	generator.Generate("./db/gen/coredb/manifest.json", "./graphql/dataloader")
}
