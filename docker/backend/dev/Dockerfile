# syntax=docker/dockerfile:1

FROM golang:1.20-bullseye

RUN apt-get update && apt-get install -y \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

COPY . /app
RUN go build -o ./bin/backend ./cmd/server/main.go

ARG VERSION
ENV GAE_VERSION=$VERSION

EXPOSE 4000
USER nobody
ENTRYPOINT ["./bin/backend"]
