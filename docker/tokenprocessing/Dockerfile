# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

ARG VERSION
ARG FFMPEG_VERSION

RUN apt-get update && \
    apt-get install -y ffmpeg=$FFMPEG_VERSION imagemagick curl xz-utils && \
    rm -rf /var/lib/apt/lists/*

# Install Node.js and npm
RUN curl -fsSL https://deb.nodesource.com/setup_16.x | bash - && \
    apt-get install -y nodejs

# Install certs
ADD root-certs /usr/local/share/ca-certificates/
RUN update-ca-certificates

# Install Go deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

# Copy the rest of the application
COPY . /app
RUN npm ci
RUN go build -o ./bin/tokenprocessing ./cmd/tokenprocessing/main.go

ENV VERSION=$VERSION

EXPOSE 6500
USER nobody
ENTRYPOINT ["./bin/tokenprocessing"]
