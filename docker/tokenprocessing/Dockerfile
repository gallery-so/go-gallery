# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

RUN apt-get update
RUN apt-get install -y ffmpeg=7:4.3.5-0+deb11u1 && rm -rf /var/lib/apt/lists/*

# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

# Install certs
ADD _deploy/root-certs /usr/local/share/ca-certificates/
RUN update-ca-certificates

COPY . /app
RUN go build -o ./bin/tokenprocessing ./cmd/tokenprocessing/main.go

ARG VERSION
ENV VERSION=$VERSION

EXPOSE 6500
USER nobody
ENTRYPOINT ["./bin/tokenprocessing"]
