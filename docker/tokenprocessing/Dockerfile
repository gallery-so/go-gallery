# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

ARG VERSION
ARG FFMPEG_VERSION

RUN apt-get update && \
    apt-get install -y ffmpeg=$FFMPEG_VERSION imagemagick && \
    rm -rf /var/lib/apt/lists/*

# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

# Install certs
ADD root-certs /usr/local/share/ca-certificates/
RUN update-ca-certificates

COPY . /app
RUN go build -o ./bin/tokenprocessing ./cmd/tokenprocessing/main.go

ENV VERSION=$VERSION

EXPOSE 6500
USER nobody
ENTRYPOINT ["./bin/tokenprocessing"]
