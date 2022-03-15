# go-gallery

## Pre

1. [Install Go](https://golang.org/doc/install)
2. [Install Docker](https://www.docker.com/products/docker-desktop)
3. [Install Docker-Compose](https://docs.docker.com/compose/install/)

## Clone and install deps

```bash
$ git clone git@github.com:gallery-so/go-gallery.git
$ cd go-gallery
$ go get -u -d ./...
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

**Deploying our Postgres image**

`make g-docker` will push a new docker image that initializes a postgres instance with our custom schema. To get access to our dockerhub, contact a core team member.

_Do not run this script if you've run the shell script for seeding NFT data._

**Working with migrations**

The `migrate` cli can be installed via brew (assuming MacOS):
```bash
brew install golang-migrate
```

Create a new migration:
```bash
migrate create -ext sql -dir db/migrations -seq <name of migration>
```

Run a migration on dev:
```bash
# Apply an up migration.
migrate -path db/migrations -database "postgresql://postgres:<dev db password here>@34.102.59.201:5432/postgres" up

# Undo the last migration.
migrate -path db/migrations -database "postgresql://postgres:<dev db password here>@34.102.59.201:5432/postgres" down 1
```

Run a migration locally:
```bash
migrate -path db/migrations -database "postgresql://postgres@localhost:5432/postgres?sslmode=disable" up
```

### Healthcheck

Verify that the server is running by calling the `/v1/health` endpoint.

```bash
$ curl localhost:4000/glry/v1/health
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
