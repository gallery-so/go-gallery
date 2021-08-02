# go-gallery

## Pre

1. [Install Go](https://golang.org/doc/install)
2. [Install Docker](https://www.docker.com/products/docker-desktop)

## Clone and install deps

```bash
$ git clone git@github.com:gallery-so/go-gallery.git
$ cd go-gallery
$ go get -u -d ./...
```

### Mongo

The app is dependent on mongo running locally. You can use the official docker container:

```bash
$ docker run -p 27017:27017 mongo:4.4.6
```

To kill the mongo process, you can close the terminal window, or run:

```bash
$ docker ps
$ docker kill <CONTAINER ID>
```

## Build - Linux

### Exec

```bash
$ go build -o ./bin/main ./cmd/server/main.go
```

This will generate a binary within `./bin/main`. To run the binary, simply:

```bash
$ ./bin/main
```

Remember that you'll need mongo running for the app to start.

### Docker

To generate a docker image of your binary, run:

```bash
# build the binary
$ go build -o ./bin/main ./cmd/server/main.go
# build the docker image
$ docker build --platform linux/amd64 -f Dockerfile --tag mikeybitcoin/gallery .
# [OPTIONAL] run the image
$ docker run mikeybitcoin/gallery
```

### Debugging Docker

This is only available on linux, assuming the docker image is built for linux:

```bash
$ docker run -it mikeybitcoin/gallery bash
```

### Other local dev scripts

Handle missing environments:

```bash
$ GLRY_AWS_SECRETS=0 AWS_REGION=us-east-1 GLRY_SENTRY_ENDPOINT=... ./main
```

## Set up - Mac

### Exec

```bash
$ go build -o ./bin/main_mac ./cmd/server/main.go
```

This will generate a binary within `./bin/main_mac`. To run the binary, simply:

```bash
$ ./bin/main_mac
```

Remember that you'll need mongo running for the app to start.

### Docker

To generate a docker image of your binary, you'll need to build for linux, since that's the OS used in the AWS machines:

```bash
# build the binary
$ GOOS=linux GOARCH=amd64 go build -o ./bin/main ./cmd/server/main.go
# build the docker image
$ docker build --platform linux/amd64 -f Dockerfile --tag mikeybitcoin/gallery .
```

## Post Setup

### Healthcheck

You can verify that the server is running by calling the `/alive` endpoint.

```bash
$ curl localhost:4000/alive
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

Add `-v` for detailed logs

## Deploy

### Publish to Dockerhub

Assuming you've followed the Docker build steps above, run:

```bash
$ docker push mikeybitcoin/gallery:latest
```

The image will be available on Dockerhub: https://hub.docker.com/r/mikeybitcoin/gallery/tags?page=1&ordering=last_updated
