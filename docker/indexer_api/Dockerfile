# syntax=docker/dockerfile:1

FROM golang:1.16-alpine AS build

# Build binaries not linked to C libs
ENV CGO_ENABLED=0

# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

COPY . /app
RUN go build -o /indexer-api ./cmd/indexer/server/main.go

FROM gcr.io/distroless/base-debian11:debug

WORKDIR /
COPY --from=build /indexer-api /indexer-api

ARG VERSION
ENV VERSION=$VERSION

EXPOSE 6000
USER nobody
ENTRYPOINT ["/indexer-api"]
