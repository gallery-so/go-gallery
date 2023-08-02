# syntax=docker/dockerfile:1

FROM golang:1.19-bullseye

ARG VERSION
ARG FFMPEG_VERSION

RUN apt-get update && \
    apt-get install -y ffmpeg=$FFMPEG_VERSION imagemagick curl xz-utils && \
    rm -rf /var/lib/apt/lists/*

ENV NVM_DIR /usr/local/nvm
ENV NODE_VERSION 20.5.0

RUN mkdir -p $NVM_DIR \
    && curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.1/install.sh | bash \
    && . $NVM_DIR/nvm.sh \
    && nvm install $NODE_VERSION \
    && nvm alias default $NODE_VERSION \
    && nvm use default

ENV NODE_PATH $NVM_DIR/versions/node/v$NODE_VERSION/lib/node_modules
ENV PATH $NVM_DIR/versions/node/v$NODE_VERSION/bin:$PATH

# Verify installation
RUN node -v
RUN npm -v

RUN apt-get update && apt-get install -y \
    dbus-x11 \
    inotify-tools

RUN apt-get update \
    && apt-get install -y wget gnupg ca-certificates \
    && wget -q -O - https://dl-ssl.google.com/linux/linux_signing_key.pub | apt-key add - \
    && sh -c 'echo "deb [arch=amd64] http://dl.google.com/linux/chrome/deb/ stable main" >> /etc/apt/sources.list.d/google.list' \
    && apt-get update \
    && apt-get install -y google-chrome-stable fonts-ipafont-gothic fonts-wqy-zenhei fonts-thai-tlwg fonts-kacst fonts-freefont-ttf libxss1 libasound2 libatk-bridge2.0-0 libatk1.0-0 libc6 libcairo2 libcups2 libdbus-1-3 libexpat1 libfontconfig1 libgbm1 libgcc1 libglib2.0-0 libgtk-3-0 libnspr4 libnss3 libpango-1.0-0 libpangocairo-1.0-0 libstdc++6 libx11-6 libx11-xcb1 libxcb1 libxcomposite1 libxcursor1 libxdamage1 libxext6 libxfixes3 libxi6 libxrandr2 libxrender1 libxtst6 lsb-release xdg-utils \
      --no-install-recommends \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r pptruser && useradd -rm -g pptruser -G audio,video pptruser

ENV PUPPETEER_SKIP_CHROMIUM_DOWNLOAD true
ENV PUPPETEER_CACHE_FOLDER=/app/puppeteer_cache
RUN mkdir -p /app/puppeteer_cache && chown -R nobody:nogroup /app/puppeteer_cache

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
USER pptruser
EXPOSE 6500
ENTRYPOINT ["./bin/tokenprocessing"]
