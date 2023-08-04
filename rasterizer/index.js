const PNG = require('pngjs').PNG;
const GIFEncoder = require('gifencoder');
const pixelmatch = require('pixelmatch');
const express = require('express');
const { Cluster } = require('puppeteer-cluster');
const app = express();

const args = [
  '--autoplay-policy=user-gesture-required',
  '--disable-background-networking',
  '--disable-background-timer-throttling',
  '--disable-backgrounding-occluded-windows',
  '--disable-breakpad',
  '--disable-client-side-phishing-detection',
  '--disable-component-update',
  '--disable-default-apps',
  '--disable-domain-reliability',
  '--disable-extensions',
  '--disable-features=AudioServiceOutOfProcess',
  '--disable-hang-monitor',
  '--disable-ipc-flooding-protection',
  '--disable-notifications',
  '--disable-offer-store-unmasked-wallet-cards',
  '--disable-popup-blocking',
  '--disable-print-preview',
  '--disable-prompt-on-repost',
  '--disable-renderer-backgrounding',
  '--disable-speech-api',
  '--disable-sync',
  '--hide-scrollbars',
  '--ignore-gpu-blacklist',
  '--metrics-recording-only',
  '--mute-audio',
  '--no-default-browser-check',
  '--no-first-run',
  '--no-pings',
  '--no-sandbox',
  '--no-zygote',
  '--password-store=basic',
  '--use-gl=swiftshader',
  '--use-mock-keychain',
  '--disable-software-rasterizer',
  '--disable-gpu',
  '--disable-gpu-compositing',
  '--disable-inotify',
];

(async () => {
  console.log('Launching cluster');
  const cluster = await Cluster.launch({
    concurrency: Cluster.CONCURRENCY_PAGE,
    maxConcurrency: 10,
    timeout: 600000,
    retryDelay: 3000,
    retryLimit: 3,
    puppeteerOptions: {
      args,
      headless: 'new',
    },
  });

  cluster.on('taskerror', (err, data, willRetry) => {
    if (willRetry) {
      console.warn(
        `Encountered an error while screenshotting ${data}. ${err.message}\nThis job will be retried`
      );
    } else {
      console.error(`Failed to screenshot ${data}: ${err.message}`);
    }
  });

  await cluster.task(async ({ page, data: url }) => {
    await page.goto(url);
    return await createAnimation(page);
  });
  app.listen(3000, async () => {
    console.log('Listening on port 3000');
  });

  app.get('/rasterize', async (req, res) => {
    if (!req.query.url) {
      res.status(400).send('no url provided');
      return;
    }
    const url = req.query.url;
    console.log('Requesting ' + url);
    try {
      const result = await cluster.execute(url);
      const j = {};
      j['png'] = result[0];
      if (result.length > 1) {
        j['gif'] = result[1];
      }
      res.status(200).send(j);
    } catch (e) {
      console.log(e);
      res.status(400).send('error' + e);
    }
  });
})();

const totalFrames = 30;
const idealDelay = 30;

process.on('unhandledRejection', (reason, p) => {
  console.error('Unhandled Rejection at:', p, 'reason:', reason);
  console.log('Unhandled Rejection at:', p, 'reason:', reason);
});

process.on('uncaughtException', (err, origin) => {
  console.error(`Caught exception: ${err}\n` + `Exception origin: ${origin}`);
  console.log(`Caught exception: ${err}\n` + `Exception origin: ${origin}`);
});

async function createAnimation(page) {
  let svgDimensions = await page.evaluate(() => {
    let svg = document.querySelector('svg');

    if (svg) {
      // If viewBox is available, use it
      if (svg.viewBox && svg.viewBox.baseVal) {
        return { width: svg.viewBox.baseVal.width, height: svg.viewBox.baseVal.height };
      }
      // If width and height are available, use them
      else if (svg.width && svg.height) {
        return { width: svg.width.baseVal.value, height: svg.height.baseVal.value };
      }
      // If none are available, throw error
      else {
        throw new Error('SVG found but no viewBox, width, or height attributes available');
      }
    } else {
      throw new Error('No SVG found');
    }
  });

  await page.setViewport(svgDimensions);

  const frames = [];

  let previousTimestamp = Date.now();
  let accumulatedDelay = 0;

  for (let i = 0; i < totalFrames; i++) {
    const frame = await page.screenshot({ fullPage: true });
    const img = PNG.sync.read(frame);
    frames.push(img);

    // Calculate the elapsed time and update the accumulated delay
    let currentTimestamp = Date.now();
    let actualDelay = currentTimestamp - previousTimestamp;
    previousTimestamp = currentTimestamp;
    accumulatedDelay += actualDelay - idealDelay;

    if (accumulatedDelay > idealDelay) {
      // Skip the next frame
      accumulatedDelay -= idealDelay;
      continue;
    }

    // Wait for the remaining time to achieve the ideal delay
    let remainingDelay = idealDelay - actualDelay;
    if (remainingDelay > 0) {
      await new Promise((resolve) => setTimeout(resolve, remainingDelay));
    }
  }

  // Compare all frames to the first one
  let isStatic = true;
  const img1 = frames[0];

  for (let i = 1; i < frames.length; i++) {
    const img2 = frames[i];
    const diff = new PNG({ width: img1.width, height: img1.height });
    const pixels = pixelmatch(img1.data, img2.data, diff.data, img1.width, img1.height, {
      threshold: 0.1,
    });
    if (pixels > 0) {
      isStatic = false;
      break;
    }
  }

  const result = [];

  const pngBuffer = PNG.sync.write(frames[0]);
  result.push(Buffer.from(pngBuffer).toString('base64'));

  if (!isStatic) {
    // If frames are different, save a gif as well
    const encoder = new GIFEncoder(frames[0].width, frames[0].height);
    const stream = encoder.createReadStream();
    let gifBuffer = Buffer.alloc(0);
    stream.on('data', (chunk) => (gifBuffer = Buffer.concat([gifBuffer, chunk])));
    stream.on('end', () => {
      result.push(gifBuffer.toString('base64'));
    });

    encoder.start();
    encoder.setRepeat(0);
    encoder.setDelay(100); // frame delay in ms
    encoder.setQuality(10); // image quality. 20 is default.

    for (let frame of frames) {
      encoder.addFrame(frame.data);
    }

    encoder.finish();
  }

  return result;
}
