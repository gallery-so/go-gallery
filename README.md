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
$ go build -o ./bin/main ./cmd/server/main.go
```

### Healthcheck

```bash
$ curl localhost:4000/alive
```
