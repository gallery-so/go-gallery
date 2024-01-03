const WebSocket = require('ws');
global.WebSocket = WebSocket;
const express = require('express');
const fetch = require('node-fetch');
const opensea = require('@opensea/stream-js');
global.fetch = fetch;

const app = express();

const client = new opensea.OpenSeaStreamClient({
  token: process.env.OPENSEA_API_KEY,
  network: opensea.Network.MAINNET,
  connectOptions: {
    transport: WebSocket,
  },
});

(async () => {
  console.log('Starting server');

  app.get('/health', async (req, res) => {
    res.send('OK');
  });

  client.onItemTransferred('*', (event) => {
    // only zora for now
    if (event.payload.chain !== 'zora') {
      return;
    }
    console.log('Item transferred: ', event);
    fetch(process.env.WEBHOOK_URL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: process.env.WEBHOOK_TOKEN,
      },
      body: JSON.stringify(event),
    })
      .then((response) => {
        console.log('Webhook response: ', response);
      })
      .catch((error) => {
        console.log('Webhook error: ', error);
      });
  });

  app.listen(3000, async () => {
    console.log('Listening on port 3000');
  });
})();

process.on('unhandledRejection', (reason, p) => {
  console.error('Unhandled Rejection at:', p, 'reason:', reason);
  console.log('Unhandled Rejection at:', p, 'reason:', reason);
});

process.on('uncaughtException', (err, origin) => {
  console.error(`Caught exception: ${err}\n` + `Exception origin: ${origin}`);
  console.log(`Caught exception: ${err}\n` + `Exception origin: ${origin}`);
});
