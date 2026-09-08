// png.mjs — minimal PNG reader for the composited-contrast gate (#2770).
//
// Decodes exactly what Playwright's `page.screenshot()` emits: 8-bit,
// non-interlaced, truecolour with or without alpha. Everything else throws
// rather than guessing.
//
// Why not a library, or ffmpeg: the tree has no image dependency and CLAUDE.md
// caps dependencies at the minimum, and ffmpeg — though used by the walkthrough
// pipeline — is NOT installed on the self-hosted runners, so relying on it would
// make the CI gate depend on a runner-image change. Node ships zlib, and the
// remainder is the five PNG filter types, which are fully specified and tested
// here against buffers built filter-by-filter.

import { inflateSync } from 'node:zlib';

const SIGNATURE = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

/** Reverses one scanline filter in place. `bpp` is bytes per pixel. */
function unfilter(type, line, prev, bpp) {
  switch (type) {
    case 0: // None
      break;
    case 1: // Sub — predict from the pixel to the left
      for (let i = bpp; i < line.length; i++) line[i] = (line[i] + line[i - bpp]) & 0xff;
      break;
    case 2: // Up — predict from the pixel above
      for (let i = 0; i < line.length; i++) line[i] = (line[i] + prev[i]) & 0xff;
      break;
    case 3: // Average — mean of left and above
      for (let i = 0; i < line.length; i++) {
        const left = i >= bpp ? line[i - bpp] : 0;
        line[i] = (line[i] + ((left + prev[i]) >> 1)) & 0xff;
      }
      break;
    case 4: // Paeth — whichever of left/above/upper-left is closest to their gradient
      for (let i = 0; i < line.length; i++) {
        const a = i >= bpp ? line[i - bpp] : 0;
        const b = prev[i];
        const c = i >= bpp ? prev[i - bpp] : 0;
        const p = a + b - c;
        const pa = Math.abs(p - a);
        const pb = Math.abs(p - b);
        const pc = Math.abs(p - c);
        const pred = pa <= pb && pa <= pc ? a : pb <= pc ? b : c;
        line[i] = (line[i] + pred) & 0xff;
      }
      break;
    default:
      throw new Error(`unsupported PNG filter type ${type}`);
  }
  return line;
}

/**
 * Decodes a PNG buffer to `{ width, height, buffer }` where `buffer` is packed
 * rgb24 — alpha is dropped, which is correct here because screenshots are fully
 * opaque and the analyzer works in RGB.
 */
export function decodePng(png) {
  if (!png.subarray(0, 8).equals(SIGNATURE)) throw new Error('not a PNG');

  let width = 0;
  let height = 0;
  let bitDepth = 0;
  let colorType = 0;
  const idat = [];

  let offset = 8;
  while (offset < png.length) {
    const length = png.readUInt32BE(offset);
    const type = png.toString('ascii', offset + 4, offset + 8);
    const data = png.subarray(offset + 8, offset + 8 + length);
    if (type === 'IHDR') {
      width = data.readUInt32BE(0);
      height = data.readUInt32BE(4);
      bitDepth = data[8];
      colorType = data[9];
      if (data[12] !== 0) throw new Error('interlaced PNG is not supported');
    } else if (type === 'IDAT') {
      idat.push(data);
    } else if (type === 'IEND') {
      break;
    }
    offset += 12 + length; // length + type + data + crc
  }

  if (bitDepth !== 8) throw new Error(`unsupported PNG bit depth ${bitDepth}`);
  if (colorType !== 2 && colorType !== 6) {
    throw new Error(`unsupported PNG colour type ${colorType} (need 2 or 6)`);
  }

  const channels = colorType === 6 ? 4 : 3;
  const raw = inflateSync(Buffer.concat(idat));
  const stride = width * channels;
  const expected = (stride + 1) * height;
  if (raw.length < expected) {
    throw new Error(`truncated PNG: ${raw.length} bytes inflated, expected ${expected}`);
  }

  const out = Buffer.alloc(width * height * 3);
  let prev = Buffer.alloc(stride);
  for (let y = 0; y < height; y++) {
    const start = y * (stride + 1);
    const filterType = raw[start];
    const line = Buffer.from(raw.subarray(start + 1, start + 1 + stride));
    unfilter(filterType, line, prev, channels);
    for (let x = 0; x < width; x++) {
      const src = x * channels;
      const dst = (y * width + x) * 3;
      out[dst] = line[src];
      out[dst + 1] = line[src + 1];
      out[dst + 2] = line[src + 2];
    }
    prev = line;
  }

  return { width, height, buffer: out };
}
