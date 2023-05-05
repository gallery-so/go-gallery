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

COPY . /app
# Enable debug_tools on dev
RUN go build -tags debug_tools -o ./bin/backend ./cmd/server/main.go

ENV GAE_VERSION=$VERSION

EXPOSE 4000
USER nobody
ENTRYPOINT ["./bin/backend"]
