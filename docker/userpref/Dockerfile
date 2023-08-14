# syntax=docker/dockerfile:1
FROM golang:1.19-bullseye

WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

COPY . /app
RUN go build -o ./bin/userpref ./cmd/userpref/main.go

ENV GAE_VERSION=$VERSION

USER nobody
ENTRYPOINT ["./bin/userpref"]
