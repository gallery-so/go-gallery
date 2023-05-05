# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

RUN apt-get update
RUN apt-get install -y && rm -rf /var/lib/apt/lists/*

# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

COPY . /app
RUN go build -o ./bin/pushnotifications ./cmd/pushnotifications/main.go

ARG VERSION
ENV VERSION=$VERSION

EXPOSE 6600
USER nobody
ENTRYPOINT ["./bin/pushnotifications"]
