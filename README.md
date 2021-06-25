# go-gallery

## Pre

1. Install Go
    - [https://golang.org/doc/install](https://golang.org/doc/install)
2. Install Docker
    - [https://www.docker.com/products/docker-desktop](https://www.docker.com/products/docker-desktop)


## Setup - Linux

1. Navigate to go-gallery directory
2. Install packages and dependencies:
```bash
$ go get -u -d ./...
```

3. Run the app:
```bash
$ go run .
```

```bash
$ GOOS=linux GOARCH=amd64 go build -o ./bin/main ./cmd/server/glry_main.go

$ docker build --platform linux/amd64 -f Dockerfile --tag mikeybitcoin/gallery .
```

start a MongoDB container to use as a local dev DB.
running it without the "-v" CLI arg will not mount a local persistent dir as a container volume,
and data will be lost once this mongo container is stopped.

```bash
$ docker run -p 27017:27017 mongo
```

start the Gallery container

```bash

# docker container debugging
$ sudo docker run -it mikeybitcoin/gallery bash

# LOCAL_DEV
$ sudo docker run --net=host mikeybitcoin/gallery
```

Local testing

```bash

$ GLRY_AWS_SECRETS=0 AWS_REGION=us-east-1 GLRY_SENTRY_ENDPOINT=... ./main
```


## Set up - Mac

1. Navigate to go-gallery directory
2. Install packages and dependencies:
```bash
$ go get -u -d ./...
```
3. Build app for local dev server
```bash
$ go build -o ./bin/main_mac ./cmd/server/glry_main.go
```
4. Run mongo instance for local db
```bash
$ docker run -p 27017:27017 mongo
```
5. Run executable to run server locally
```
$ cd bin; ./main_mac
```


## Post set up

### Healthcheck
You can verify that the server is running by calling the `/alive` endpoint.

```bash
$ curl localhost:4000/alive
```

