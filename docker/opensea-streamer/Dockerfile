FROM node:latest

WORKDIR /app
COPY package*.json ./

USER root

RUN npm install

COPY ./opensea-streamer/index.js ./

EXPOSE 3000
CMD [ "node", "index.js" ]
