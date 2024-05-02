# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

ARG VERSION

# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

COPY . /app
RUN go build -o ./bin/kafka-streamer ./kafka-streamer

ENV GAE_VERSION=$VERSION

EXPOSE 3000
USER nobody
ENTRYPOINT ["./bin/kafka-streamer"]
