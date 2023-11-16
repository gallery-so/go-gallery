# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

ARG VERSION
# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

COPY . /app
RUN go build -o ./bin/activity_stats ./cmd/activity_stats/main.go

ENV GAE_VERSION=$VERSION

EXPOSE 6700
USER nobody
ENTRYPOINT ["./bin/activity_stats"]
