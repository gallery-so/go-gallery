# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

RUN apt-get update && \
    apt-get install -y ffmpeg imagemagick curl xz-utils && \
    rm -rf /var/lib/apt/lists/*

# Install Node.js and npm
RUN curl -fsSL https://deb.nodesource.com/setup_16.x | bash - && \
    apt-get install -y nodejs

# Install Go deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

COPY . /app
RUN npm ci
RUN go build -o ./bin/reprocess ./cmd/reprocess/main.go

ARG VERSION
ENV GAE_VERSION=$VERSION

USER nobody
ENTRYPOINT ["./bin/reprocess"]
