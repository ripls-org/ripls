// Tests for the minimal PNG reader (#2770).
//
// Fixtures are built here rather than committed, so each of the five filter
// types is exercised deliberately instead of depending on whichever ones an
// encoder happened to choose. A decoder that silently mishandles Paeth would
// otherwise produce plausible-looking pixels and a confidently wrong contrast
// number.

import test from 'node:test';
import assert from 'node:assert';
import { deflateSync } from 'node:zlib';

import { decodePng } from './png.mjs';

const SIGNATURE = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

function crc32(buf) {
  let c = ~0;
  for (const byte of buf) {
    c ^= byte;
    for (let k = 0; k < 8; k++) c = (c >>> 1) ^ (0xedb88320 & -(c & 1));
  }
  return ~c >>> 0;
}

function chunk(type, data) {
  const len = Buffer.alloc(4);
  len.writeUInt32BE(data.length);
  const body = Buffer.concat([Buffer.from(type, 'ascii'), data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(body));
  return Buffer.concat([len, body, crc]);
}

/**
 * Builds a PNG from raw pixels, applying `filterTypes[y]` to each scanline.
 * Filters are applied in the forward direction so the decoder must reverse
 * them correctly to recover `pixels`.
 */
function encodePng(width, height, pixels, channels, filterTypes) {
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(width, 0);
  ihdr.writeUInt32BE(height, 4);
  ihdr[8] = 8;
  ihdr[9] = channels === 4 ? 6 : 2;
  ihdr[10] = 0;
  ihdr[11] = 0;
  ihdr[12] = 0;

  const stride = width * channels;
  const raw = [];
  let prev = Buffer.alloc(stride);
  for (let y = 0; y < height; y++) {
    const line = Buffer.from(pixels.subarray(y * stride, (y + 1) * stride));
    const type = filterTypes[y % filterTypes.length];
    const filtered = Buffer.alloc(stride);
    for (let i = 0; i < stride; i++) {
      const a = i >= channels ? line[i - channels] : 0;
      const b = prev[i];
      const c = i >= channels ? prev[i - channels] : 0;
      let pred = 0;
      if (type === 1) pred = a;
      else if (type === 2) pred = b;
      else if (type === 3) pred = (a + b) >> 1;
      else if (type === 4) {
        const p = a + b - c;
        const pa = Math.abs(p - a);
        const pb = Math.abs(p - b);
        const pc = Math.abs(p - c);
        pred = pa <= pb && pa <= pc ? a : pb <= pc ? b : c;
      }
      filtered[i] = (line[i] - pred) & 0xff;
    }
    raw.push(Buffer.concat([Buffer.from([type]), filtered]));
    prev = line;
  }

  return Buffer.concat([
    SIGNATURE,
    chunk('IHDR', ihdr),
    chunk('IDAT', deflateSync(Buffer.concat(raw))),
    chunk('IEND', Buffer.alloc(0)),
  ]);
}

/** A gradient with structure in every channel, so filter bugs surface. */
function samplePixels(width, height, channels) {
  const buf = Buffer.alloc(width * height * channels);
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      const i = (y * width + x) * channels;
      buf[i] = (x * 7 + y * 3) & 0xff;
      buf[i + 1] = (x * 13 + y * 29) & 0xff;
      buf[i + 2] = (x * 31 + y * 17) & 0xff;
      if (channels === 4) buf[i + 3] = 255;
    }
  }
  return buf;
}

for (const [name, filters] of [
  ['None', [0]],
  ['Sub', [1]],
  ['Up', [2]],
  ['Average', [3]],
  ['Paeth', [4]],
  ['mixed per scanline', [0, 1, 2, 3, 4]],
]) {
  test(`decodePng: round-trips RGBA with the ${name} filter`, () => {
    const [w, h] = [9, 7];
    const pixels = samplePixels(w, h, 4);
    const decoded = decodePng(encodePng(w, h, pixels, 4, filters));
    assert.strictEqual(decoded.width, w);
    assert.strictEqual(decoded.height, h);
    for (let p = 0; p < w * h; p++) {
      for (let c = 0; c < 3; c++) {
        assert.strictEqual(
          decoded.buffer[p * 3 + c],
          pixels[p * 4 + c],
          `pixel ${p} channel ${c} with filter ${name}`,
        );
      }
    }
  });
}

test('decodePng: handles truecolour without alpha', () => {
  const [w, h] = [5, 4];
  const pixels = samplePixels(w, h, 3);
  const decoded = decodePng(encodePng(w, h, pixels, 3, [0, 1, 2, 3, 4]));
  assert.deepStrictEqual(decoded.buffer, pixels);
});

test('decodePng: output is packed rgb24 of exactly the right size', () => {
  const decoded = decodePng(encodePng(6, 3, samplePixels(6, 3, 4), 4, [0]));
  assert.strictEqual(decoded.buffer.length, 6 * 3 * 3);
});

test('decodePng: rejects what it cannot faithfully decode', () => {
  assert.throws(() => decodePng(Buffer.alloc(32)), /not a PNG/);

  // Interlaced — decoding it as progressive would silently scramble pixels.
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(2, 0);
  ihdr.writeUInt32BE(2, 4);
  ihdr[8] = 8;
  ihdr[9] = 6;
  ihdr[12] = 1;
  const interlaced = Buffer.concat([
    SIGNATURE,
    chunk('IHDR', ihdr),
    chunk('IDAT', deflateSync(Buffer.alloc(20))),
    chunk('IEND', Buffer.alloc(0)),
  ]);
  assert.throws(() => decodePng(interlaced), /interlaced/);

  // 16-bit depth.
  const deep = Buffer.from(ihdr);
  deep[8] = 16;
  deep[12] = 0;
  assert.throws(
    () =>
      decodePng(
        Buffer.concat([
          SIGNATURE,
          chunk('IHDR', deep),
          chunk('IDAT', deflateSync(Buffer.alloc(40))),
          chunk('IEND', Buffer.alloc(0)),
        ]),
      ),
    /bit depth/,
  );
});

test('decodePng: reassembles pixel data split across multiple IDAT chunks', () => {
  // Real encoders split IDAT; a decoder that reads only the first chunk works
  // on small fixtures and fails on full-size screenshots.
  const [w, h] = [8, 6];
  const pixels = samplePixels(w, h, 4);
  const full = encodePng(w, h, pixels, 4, [0]);
  const decodedWhole = decodePng(full);

  const stride = w * 4;
  const raw = [];
  for (let y = 0; y < h; y++) {
    raw.push(Buffer.from([0]), pixels.subarray(y * stride, (y + 1) * stride));
  }
  const compressed = deflateSync(Buffer.concat(raw));
  const mid = Math.floor(compressed.length / 2);

  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0);
  ihdr.writeUInt32BE(h, 4);
  ihdr[8] = 8;
  ihdr[9] = 6;
  const split = Buffer.concat([
    SIGNATURE,
    chunk('IHDR', ihdr),
    chunk('IDAT', compressed.subarray(0, mid)),
    chunk('IDAT', compressed.subarray(mid)),
    chunk('IEND', Buffer.alloc(0)),
  ]);
  assert.deepStrictEqual(decodePng(split).buffer, decodedWhole.buffer);
});
