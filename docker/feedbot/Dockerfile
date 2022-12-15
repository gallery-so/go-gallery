# syntax=docker/dockerfile:1

FROM golang:1.16-bullseye

RUN apt-get update
RUN apt-get install -y && rm -rf /var/lib/apt/lists/*

# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

COPY . /app
RUN go build -o ./bin/feedbot ./cmd/feedbot/main.go

ARG VERSION
ENV GAE_VERSION=$VERSION

EXPOSE 4124
USER nobody
ENTRYPOINT ["./bin/feedbot"]
