# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

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
