# syntax=docker/dockerfile:1

FROM golang:1.16-bullseye

RUN apt-get update && apt-get install -y \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

COPY . /app
RUN go build -o ./bin/feed ./cmd/feed/main.go

ARG VERSION
ENV GAE_VERSION=$VERSION

EXPOSE 4124
USER nobody
ENTRYPOINT ["./bin/feed"]
