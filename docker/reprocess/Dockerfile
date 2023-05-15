# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

RUN apt-get update && \
    apt-get install -y ffmpeg imagemagick && \
    rm -rf /var/lib/apt/lists/*


# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

COPY . /app
RUN go build -o ./bin/reprocess ./cmd/reprocess/main.go

ARG VERSION
ENV GAE_VERSION=$VERSION

USER nobody
ENTRYPOINT ["./bin/reprocess"]
