# go-gallery

## Pre

1. [Install Go](https://golang.org/doc/install)
2. [Install Docker](https://www.docker.com/products/docker-desktop)
3. [Install Docker-Compose](https://docs.docker.com/compose/install/)

## Clone and install deps

```bash
$ git clone git@github.com:gallery-so/go-gallery.git
$ cd go-gallery
$ go get -u=patch -d ./...
```

## Setup (Mac)

```bash
$ go build -o ./bin/main ./cmd/server/main.go
```

This will generate a binary within `./bin/main`. To run the binary, simply:

```bash
$ ./bin/main
```

### Redis and Postgres

The app will connect to a local redis and local postgres instance by default. To spin it up, you can use the docker commands below.

**[Optional] Shell script to seed NFT data**

If you want to seed your local database with real, indexed data from our dev or production clusters, you can "prep" your environment using the following bash script. Running this won't execute the import itself, but rather trigger the import when you run the later docker commands. _As a pre-requisite, you must have access to `_encrypted_deploy` in order to access the dev / prod clusters_.

Note that if you run the following command, don't run `make g-docker` and upload the image to Dockerhub. This will expose the locally migrated data to the public. You can avoid this by opening a
new shell. More on `make g-docker` further below.

Finally: if you are using bash/sh instead of zsh, change the first line of the `_import_env.sh` file to match your shell.

```bash
$ source ./_import_env.sh <path to dev/prod backend app.yaml> <address of dev/prod wallet to import data>
```

**Docker commands**

Build the docker containers. If you ran the above shell script, the seed script will be executed. You can re-run this script in the future if you want the latest data:

```bash
$ make docker-build
```

Run the docker containers:

```bash
$ make docker-start
```

To remove running redis and postgres instance:

```bash
$ make docker-stop
```

**Working with migrations**

The `migrate` cli can be installed via brew (assuming MacOS):

```bash
brew install golang-migrate
```

Create a new migration:

```bash
# New migration for the backend db
migrate create -ext sql -dir db/migrations/core -seq <name of migration>

# New migration for the indexer db
migrate create -ext sql -dir db/migrations/indexer -seq <name of migration>
```

Run a migration locally:

```bash
# Run all migrations for the local backend db
make migrate-coredb

# Run all migrations for the local indexer db
make migrate-indexerdb
```

Run a migration on dev backend db:

```bash
# Apply an up migration to the backend db
migrate -path db/migrations/core -database "postgresql://postgres:<dev db password here>@34.102.59.201:5432/postgres" up

# Undo the last migration to the backend db
migrate -path db/migrations/core -database "postgresql://postgres:<dev db password here>@34.102.59.201:5432/postgres" down 1
```

Run a migration on the indexer db:

```bash
# Apply an up migration to the indexer db
migrate -path db/migrations/indexer -database "postgresql://postgres:<indexer db password here>@<indexer db ip>:5432/postgres" up

# Undo the last migration to the indexer db
migrate -path db/migrations/indexer -database "postgresql://postgres:<indexer db password here>@<indexer db ip>:5432/postgres" down 1
```

### Healthcheck

Verify that the server is running:

```bash
$ curl localhost:4000/alive
```

This is available for live environments:

```bash
$ curl api.gallery.so/alive
```
```

## Testing

Run all tests in current directory and all of its subdirectories:

```bash
$ go test ./...
```

Run all tests in subdirectory (e.g. /server):

```bash
$ go test ./server/...
```

Run a specific test by passing including the `-run` flag. The example will run GraphQL tests under the `TestMain` suite that start with "should get trending".
```bash
go test -run=TestMain/"test GraphQL"/"should get trending" ./graphql
```

Add `-v` for detailed logs.

Skip longer running tests with the `-short` flag:

```bash
go test -short
```

### Running locally with live data

If you have access to the `_encrypted_local` file, you can run the server locally with live data. This is useful for testing the server locally with real data.

For example, to run the server locally with live data from the `dev` environment, run the following command:

```bash
go run cmd/server/main.go dev
```

To run the indexer server connected to production, run the following command:

```bash
go run cmd/indexer/server/main.go prod
```

When testing the indexer locally, you may want to sync log data from the prod to dev bucket. You can do this by running the sync command below. This command can take a few minutes depending on when the buckets were last synced.

```bash
# Do not switch the order of the buckets! Doing so may overwrite prod data.
gsutil -m rsync -r gs://prod-eth-token-logs gs://dev-eth-token-logs
```

### Root CAs for RPC

These are the added certificates that are included in the `_deploy` folder. They are used to verify the SSL certificates of
various HTTPS endpoints.

- `sectigo.crt` - [Sectigo RSA Domain Validation Secure Server CA](https://support.sectigo.com/articles/Knowledge/Sectigo-Intermediate-Certificates) for example [vibes.art](https://vibes.art/vibes/jpg/8095.jpg)
