# go-gallery

## Setup

### Install go

https://golang.org/doc/install

### Start app

Install packages and dependencies:

```bash
$ go get -u -d ./...
```

Run the app:

```bash
$ go run .
```

```bash
$ GOOS=linux GOARCH=amd64 go build -o ./bin/main ./cmd/server/glry_main.go

$ sudo docker build --platform linux/amd64 -f Dockerfile --tag mikeybitcoin/gallery .
```

start a MongoDB container to use as a local dev DB.
running it without the "-v" CLI arg will not mount a local persistent dir as a container volume,
and data will be lost once this mongo container is stopped.

```bash
$ sudo docker run -p 27017:27017 mongo
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

### Healthcheck

```bash
$ curl localhost:4000/alive
```


//// TEMPORARY instructions
## Pre

1. Install Go
    - [https://golang.org/doc/install#download](https://golang.org/doc/install#download)
2. Install Docker
    - [https://www.docker.com/products/docker-desktop](https://www.docker.com/products/docker-desktop)
3. In order to deploy the backend to AWS, you need an AWS login. Here are instructions for the admin to create a new account.
    1. Go to IAM
    2. Go to Users tab
    3. Click Add User

## High level

3 steps to deploying the backend

1. Local dev environment
    - so you can make changes and test changes
2. Push to DockerHub
    - repo to hold the backend
3. Deploy to AWS
    - going to AWS console to pull the latest build from dockerhub

## Set up

1. Open go-gallery directory in terminal
2. `go get -u -d ./...`

## 1) Local Dev

1. Build for local dev. This sets up the 
    - `go build -o ./bin/main_mac ./cmd/server/glry_main.go`
        - or if alias is set up `gbuild`
2. Run mongo instance
    - `docker run -p 27017:27017 mongo`
3. Run executable to run server
    - `cd bin; ./main_mac`
4. When ready to push to Dockerhub, run this so that it's executable in a linux and amd64 environment (which is what docker is set up to run)
    - `GOOS=linux GOARCH=amd64 go build -o ./bin/main ./cmd/server/glry_main.go`
    - or if alias is set up: `gdbuild`
