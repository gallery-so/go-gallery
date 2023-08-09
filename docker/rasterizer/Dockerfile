FROM ghcr.io/puppeteer/puppeteer:latest

WORKDIR /app
COPY package*.json ./

USER root
RUN chown -R pptruser:pptruser /app
USER pptruser

RUN npm install

COPY ./rasterizer/index.js ./

EXPOSE 3000
CMD [ "node", "index.js" ]
