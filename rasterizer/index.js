const PNG = require('pngjs').PNG;
const GIFEncoder = require('gifencoder');
const pixelmatch = require('pixelmatch');
const express = require('express');
const { Cluster } = require('puppeteer-cluster');
const app = express();
const fs = require('fs');

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
    concurrency: Cluster.CONCURRENCY_CONTEXT,
    maxConcurrency: 5,
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

  // this function defines what is run when you call cluster.execute
  await cluster.task(async ({ page, data: url }) => {
    await page.goto(url);
    return await createAnimation(page);
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
        console.log(`Returning ${j['gif'].length} bytes for gif: ${url}`);
      }
      console.log(`Returning ${j['png'].length} bytes for thumbnail: ${url}`);
      res.status(200).send(j);
    } catch (e) {
      console.log(e);
      res.status(400).send('error' + e);
    }
  });

  app.listen(3000, async () => {
    console.log('Listening on port 3000');
  });
})();

// total screenshots
const totalFrames = 10;
// ideal delay between screenshots in ms
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
    if (!svg) throw new Error('No SVG found');

    let width, height;

    let viewBoxAttr = svg.getAttribute('viewBox');
    if (viewBoxAttr) {
      let viewBoxValues = viewBoxAttr.split(' ');
      width = parseFloat(viewBoxValues[2]);
      height = parseFloat(viewBoxValues[3]);
    } else if (svg.width.baseVal && svg.height.baseVal) {
      width = svg.width.baseVal.value;
      height = svg.height.baseVal.value;
    } else {
      throw new Error('SVG found but no viewBox or baseVal for width/height available');
    }

    // Scale the SVG to a fixed height
    const fixedHeight = 800;
    const aspectRatio = width / height;
    const scaledWidth = fixedHeight * aspectRatio;

    return {
      width: scaledWidth,
      height: fixedHeight,
    };
  });

  console.log(`SVG dimensions for ${page.url()}: ${JSON.stringify(svgDimensions)}`);

  await page.setViewport({
    width: Math.ceil(svgDimensions.width),
    height: svgDimensions.height, // Fixed height
    deviceScaleFactor: 1, // Adjust this as needed for higher resolution screenshots
  });

  const svgElement = await page.$('svg');
  if (!svgElement) throw new Error('SVG element not found');

  const frames = [];

  let previousTimestamp = Date.now();
  let accumulatedDelay = 0;

  // Take a screenshot of each frame and try to take evenly spaced screenshots by comparing times before and after screenshotting
  for (let i = 0; i < totalFrames; i++) {
    const frame = await svgElement.screenshot();
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

  let isStatic = true;
  const img1 = frames[0];

  // compare each frame to the first frame
  // if there is any difference ever, it is animated
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

  // uncomment this line if you'd like to see the result of the first frame
  // fs.writeFileSync('test.png', pngBuffer);

  result.push(Buffer.from(pngBuffer).toString('base64'));

  if (!isStatic) {
    console.log('Animated SVG detected for ' + page.url());
    const encoder = new GIFEncoder(frames[0].width, frames[0].height);
    const stream = encoder.createReadStream();
    let gifBuffer = Buffer.alloc(0);

    encoder.start();
    encoder.setRepeat(0);
    encoder.setDelay(100); // frame delay in ms
    encoder.setQuality(10); // image quality. 20 is default.

    for (let frame of frames) {
      encoder.addFrame(frame.data);
    }

    encoder.finish();

    stream.on('data', (chunk) => (gifBuffer = Buffer.concat([gifBuffer, chunk])));
    stream.on('end', () => {
      result.push(gifBuffer.toString('base64'));
    });
  }

  // result is an array of base64 encoded strings, min length 1, max length 2, first element is always the png, second is the gif if it exists
  return result;
}
