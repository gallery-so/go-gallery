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

RUN apt-get update \
    && apt-get install -y wget gnupg \
    && wget -q -O - https://dl-ssl.google.com/linux/linux_signing_key.pub | apt-key add - \
    && sh -c 'echo "deb [arch=amd64] http://dl.google.com/linux/chrome/deb/ stable main" >> /etc/apt/sources.list.d/google.list' \
    && apt-get update \
    && apt-get install -y google-chrome-stable fonts-ipafont-gothic fonts-wqy-zenhei fonts-thai-tlwg fonts-kacst fonts-freefont-ttf libxss1 \
      --no-install-recommends \
    && rm -rf /var/lib/apt/lists/*

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

# Run everything after as non-privileged user.

# Tell Puppeteer to skip installing Chrome. We'll be using the installed package.
ENV PUPPETEER_SKIP_CHROMIUM_DOWNLOAD true

ENV VERSION=$VERSION
USER nobody
EXPOSE 6500
ENTRYPOINT ["./bin/tokenprocessing"]
