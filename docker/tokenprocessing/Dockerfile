# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

ARG VERSION
ARG FFMPEG_VERSION

RUN apt-get update && \
    apt-get install -y ffmpeg=$FFMPEG_VERSION imagemagick curl xz-utils && \
    rm -rf /var/lib/apt/lists/*

# Install certs
ADD root-certs /usr/local/share/ca-certificates/
RUN update-ca-certificates

# Install Go deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

# Copy the rest of the application
COPY . /app
RUN go build -o ./bin/tokenprocessing ./cmd/tokenprocessing/main.go

ENV VERSION=$VERSION
USER nobody
EXPOSE 6500
ENTRYPOINT ["./bin/tokenprocessing"]
