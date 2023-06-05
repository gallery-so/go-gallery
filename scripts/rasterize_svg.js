const puppeteer = require('puppeteer');
const PNG = require('pngjs').PNG;
const GIFEncoder = require('gifencoder');
const pixelmatch = require('pixelmatch');
const sharp = require('sharp');

const totalFrames = 30;
const delay = 33; // 30fps

async function createAnimation() {
  const url = process.argv[2];
  const browser = await puppeteer.launch();
  const page = await browser.newPage();

  await page.evaluate(async () => {
    let svg = document.querySelector('svg');

    if (svg) {
      svg.style.width = '100%';
      svg.style.height = '100%';
      let viewport = svg.getBoundingClientRect();
      await page.setViewport({ width: viewport.width, height: viewport.height });
    }
  });

  await page.goto(url);

  const frames = [];
  const idealDelay = 33.3; // Delay in milliseconds for ~30 FPS
  let previousTimestamp = Date.now();
  let accumulatedDelay = 0;

  for (let i = 0; i < totalFrames; i++) {
    const frame = await page.screenshot({ fullPage: true });
    frames.push(frame);

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
  const diff = new PNG({ width: frames[0].width, height: frames[0].height });
  for (let i = 1; i < frames.length; i++) {
    const pixels = pixelmatch(
      frames[0].data,
      frames[i].data,
      diff.data,
      frames[0].width,
      frames[0].height,
      { threshold: 0.1 }
    );
    if (pixels != 0) {
      isStatic = false;
      break;
    }
  }

  if (isStatic) {
    // If all frames are identical, save as PNG
    const pngBuffer = PNG.sync.write(frames[0]);
    console.log('PNG');
    console.log(Buffer.from(pngBuffer).toString('base64'));
  } else {
    // If frames are different, save as GIF
    const encoder = new GIFEncoder(frames[0].width, frames[0].height);

    encoder.start();
    encoder.setRepeat(0);
    encoder.setDelay(delay); // frame delay in ms
    encoder.setQuality(10); // image quality. 20 is default.

    for (let frame of frames) {
      const rgba = await sharp(frame, {
        raw: { width: svgDimensions.width, height: svgDimensions.height, channels: 4 },
      })
        .ensureAlpha()
        .raw()
        .toBuffer();
      encoder.addFrame(rgba);
    }

    encoder.finish();

    const gifBuffer = encoder.out.getData();

    console.log('GIF');

    console.log(gifBuffer.toString('base64'));
  }

  await browser.close();
}

createAnimation();
