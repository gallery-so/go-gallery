# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

# Install deps
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download

COPY . /app
RUN go build -o ./bin/emails ./cmd/emails/main.go

ARG VERSION
ENV VERSION=$VERSION

EXPOSE 5500
USER nobody
ENTRYPOINT ["./bin/emails"]
