# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

ARG VERSION
ARG FFMPEG_VERSION

RUN apt-get update
RUN apt-get install -y ffmpeg=$FFMPEG_VERSION && rm -rf /var/lib/apt/lists/*

# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

# Install certs
COPY root-certs/sectigo.crt /usr/local/share/ca-certificates/sectigo.crt
RUN update-ca-certificates

COPY . /app
RUN go build -o ./bin/indexer ./cmd/indexer/main.go

ENV VERSION=$VERSION

EXPOSE 6000
USER nobody
ENTRYPOINT ["./bin/indexer", "server", "--enable-rpc"]
