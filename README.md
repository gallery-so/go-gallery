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

## Testing services via Cloud Tasks

We use [aertje/cloud-tasks-emulator](https://github.com/aertje/cloud-tasks-emulator) to emulate running Cloud Tasks locally. The emulator is added as a submodule to the repo.

See [targeting services](https://github.com/aertje/cloud-tasks-emulator#targeting-services) for more info on the setup. A typical local setup looks something like below:

```
+-----------+
|     ui    |
|  (:3000)  |
+-----|-----+
      |
      |
      |
+-----V-----+     +-------------+     +-----------+        +------------+
|   server  ------> cloud tasks ------>   nginx   --------->   feed     |
|  (:4000)  |     |   (:8123)   |     |  (:8080)  |        |  (:4124)   |
+-----------+     +-------------+     +-----|-----+        +------------+
                                            |
                                            |              +------------+
                                            +-------------->  feedbot   |
                                            |              |  (:4123)   |
                                            |              +------------+
                                            |
                                            |              +-------------------+
                                            +-------------->  mediaprocessing  |
                                            |              |      (:6500)      |
                                            |              +-------------------+
                                            |
                                            |              +---------------+
                                            +-------------->  indexer-api  |
                                                           |    (:6000)    |
                                                           +---------------+
```

To get started:

```bash
# If first time pulling down the submodule
git submodule update --init --recursive

# To update the submodule afterwards
git pull --recurse-submodules
```

After pulling the repo, start the services you're interested in running:

```bash
# Start the cloud task emulator and docker dependencies
make cloud-tasks

# Start each of the services needed in separate sessions
go run cmd/feed/main.go    # Start the feed
go run cmd/feedbot/main.go # Start the feedbot

# Start the backend
go run cmd/server/main.go
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

Run a specific test by passing the name as an option:

```bash
go test -run {testName}
```

Add `-v` for detailed logs.

Skip longer running tests with the `-short` flag:

```bash
go test -short
```

### Integration Testing

Run integration tests against ethereum mainnet:

```bash
go test -run TestIntegrationTest ./server/... -args -chain ethereum -chainID 1
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
