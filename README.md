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

The first time you run the docker-containers on your machine, you will need to run the following command(s):

_note_: If you have access to the \_encrypted_deploy files and would like to use indexed data locally, run the following commands:

_second note_: If you are using bash/sh instead of zsh, change the first line of the `_import_env.sh` file to match your shell.

_third note_: If you do this, make sure not to run `make g-docker` and upload the image to Docker Hub. That will expose user data to the public. Before you do push to docker-hub, open a new shell.

```bash
$ source ./_import_env.sh <path to dev/prod backend app.yaml> <username of dev/prod user you want to import data for>
```

```bash
$ make docker-build
```

To run the docker containers, run the following command:

```bash
$ make docker-start
```

To remove running redis and postgres instance:

```bash
$ make docker-stop
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
