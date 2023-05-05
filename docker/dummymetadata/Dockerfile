# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

# Install certs
ADD root-certs /usr/local/share/ca-certificates/
RUN update-ca-certificates

COPY . /app
RUN go build -o ./bin/dummymetadata ./cmd/dummymetadata/main.go

ARG VERSION
ENV VERSION=$VERSION

EXPOSE 8500
USER nobody
ENTRYPOINT ["./bin/dummymetadata"]
