# go-gallery

## Setup

### Install go

https://golang.org/doc/install

### Start app

Install packages and dependencies:

```bash
$ go get
```

Run the app:

```bash
$ go run .
```

```bash
$ go build -o ./bin/main ./cmd/server/glry_main.go

$ sudo docker build -f Dockerfile --tag mikeybitcoin/gallery .
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
