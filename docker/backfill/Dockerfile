# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

RUN apt-get update
RUN apt-get install -y ffmpeg=7:4.3.5-0+deb11u1 && rm -rf /var/lib/apt/lists/*

# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

COPY . /app
RUN go build -o ./bin/backfill ./cmd/optimization_backfill/main.go

ARG VERSION
ENV GAE_VERSION=$VERSION

USER nobody
ENTRYPOINT ["./bin/backfill"]
