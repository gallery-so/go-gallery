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

The app will connect to a local redis and local postgres instance by default. To spin it up, you can use the official docker containers:

```bash
$ docker-compose up -d
```

To remove running redis and postgres instance:

```bash
$ docker-compose down
```

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

# Undo a migration.
migrate -path db/migrations -database "postgresql://postgres:<dev db password here>@34.102.59.201:5432/postgres" down
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
