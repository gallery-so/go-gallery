const puppeteer = require('puppeteer');
const PNG = require('pngjs').PNG;
const GIFEncoder = require('gifencoder');
const pixelmatch = require('pixelmatch');

const totalFrames = 10;

async function captureFrame(page, delay) {
  await page.waitForTimeout(delay);
  return await page.screenshot({ fullPage: true });
}

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
  for (let i = 0; i < totalFrames; i++) {
    const frame = await captureFrame(page, 100); // Capture frame every 100ms
    frames.push(PNG.sync.read(frame));
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
    const stream = encoder.createReadStream();

    let gifBuffer = Buffer.alloc(0);
    stream.on('data', (chunk) => (gifBuffer = Buffer.concat([gifBuffer, chunk])));
    stream.on('end', () => {
      console.log('GIF');
      console.log(gifBuffer.toString('base64'));
    });

    encoder.start();
    encoder.setRepeat(0); // 0 for repeat, -1 for no-repeat
    encoder.setDelay(500); // frame delay in ms
    encoder.setQuality(10); // image quality. 20 is default.

    frames.forEach((frame) => {
      encoder.addFrame(frame.data);
    });

    encoder.finish();
  }

  await browser.close();
}

createAnimation();
