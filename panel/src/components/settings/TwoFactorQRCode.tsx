'use client';

/**
 * QR code rendering for two-factor enrolment, with no dependency.
 *
 * The otpauth:// URI has to reach a phone, and typing a 32 character base32
 * secret by hand is how enrolment goes wrong. A QR code is therefore not a
 * nicety here. Adding a library for it would mean shipping a third-party
 * package into the one screen that hands out authentication secrets, so the
 * encoder is written out: byte mode, error correction level L, versions 1 to
 * 10, which covers any otpauth URI the panel can produce.
 *
 * This is ISO/IEC 18004. The pieces are: bit stream assembly, Reed-Solomon
 * error correction over GF(256), block interleaving, function pattern
 * placement, the eight data masks with the standard penalty scoring, and the
 * BCH-protected format and version information.
 */

import * as React from 'react';

// ---------------------------------------------------------------------------
// Version table, error correction level L
//
// Per version: total codewords, EC codewords per block, and the block layout.
// Level L is chosen deliberately: an otpauth URI is scanned from a bright
// screen at arm's length, not off a scuffed parcel, so capacity is worth more
// than recovery from damage.
// ---------------------------------------------------------------------------

interface VersionSpec {
  version: number;
  totalCodewords: number;
  ecPerBlock: number;
  group1Blocks: number;
  group1Data: number;
  group2Blocks: number;
  group2Data: number;
}

const VERSIONS: VersionSpec[] = [
  { version: 1, totalCodewords: 26, ecPerBlock: 7, group1Blocks: 1, group1Data: 19, group2Blocks: 0, group2Data: 0 },
  { version: 2, totalCodewords: 44, ecPerBlock: 10, group1Blocks: 1, group1Data: 34, group2Blocks: 0, group2Data: 0 },
  { version: 3, totalCodewords: 70, ecPerBlock: 15, group1Blocks: 1, group1Data: 55, group2Blocks: 0, group2Data: 0 },
  { version: 4, totalCodewords: 100, ecPerBlock: 20, group1Blocks: 1, group1Data: 80, group2Blocks: 0, group2Data: 0 },
  { version: 5, totalCodewords: 134, ecPerBlock: 26, group1Blocks: 1, group1Data: 108, group2Blocks: 0, group2Data: 0 },
  { version: 6, totalCodewords: 172, ecPerBlock: 18, group1Blocks: 2, group1Data: 68, group2Blocks: 0, group2Data: 0 },
  { version: 7, totalCodewords: 196, ecPerBlock: 20, group1Blocks: 2, group1Data: 78, group2Blocks: 0, group2Data: 0 },
  { version: 8, totalCodewords: 242, ecPerBlock: 24, group1Blocks: 2, group1Data: 97, group2Blocks: 0, group2Data: 0 },
  { version: 9, totalCodewords: 292, ecPerBlock: 30, group1Blocks: 2, group1Data: 116, group2Blocks: 0, group2Data: 0 },
  { version: 10, totalCodewords: 346, ecPerBlock: 18, group1Blocks: 2, group1Data: 68, group2Blocks: 2, group2Data: 69 },
];

// Alignment pattern centre coordinates per version. Version 1 has none.
const ALIGNMENT_POSITIONS: number[][] = [
  [], [], [6, 18], [6, 22], [6, 26], [6, 30], [6, 34],
  [6, 22, 38], [6, 24, 42], [6, 26, 46], [6, 28, 50],
];

// Remainder bits appended after the interleaved codewords, by version.
const REMAINDER_BITS: number[] = [0, 0, 7, 7, 7, 7, 7, 0, 0, 0, 0];

// Format bits for error correction level L.
const ECC_L_FORMAT_BITS = 1;

function specFor(version: number): VersionSpec {
  const spec = VERSIONS[version - 1];
  if (!spec) throw new Error(`unsupported QR version ${version}`);
  return spec;
}

function dataCodewords(spec: VersionSpec): number {
  return spec.totalCodewords - spec.ecPerBlock * (spec.group1Blocks + spec.group2Blocks);
}

// Byte mode character count indicator: 8 bits below version 10, 16 from 10 up.
function countBits(version: number): number {
  return version < 10 ? 8 : 16;
}

// ---------------------------------------------------------------------------
// GF(256) arithmetic for Reed-Solomon, primitive polynomial 0x11D
// ---------------------------------------------------------------------------

const GF_EXP = new Uint8Array(512);
const GF_LOG = new Uint8Array(256);

(function buildTables() {
  let x = 1;
  for (let i = 0; i < 255; i++) {
    GF_EXP[i] = x;
    GF_LOG[x] = i;
    x <<= 1;
    if (x & 0x100) x ^= 0x11d;
  }
  for (let i = 255; i < 512; i++) {
    GF_EXP[i] = GF_EXP[i - 255];
  }
})();

function gfMul(a: number, b: number): number {
  if (a === 0 || b === 0) return 0;
  return GF_EXP[GF_LOG[a] + GF_LOG[b]];
}

/** generatorPoly returns the RS generator polynomial of the given degree. */
function generatorPoly(degree: number): Uint8Array {
  let poly = Uint8Array.from([1]);
  for (let i = 0; i < degree; i++) {
    const next = new Uint8Array(poly.length + 1);
    for (let j = 0; j < poly.length; j++) {
      next[j] ^= poly[j];
      next[j + 1] ^= gfMul(poly[j], GF_EXP[i]);
    }
    poly = next;
  }
  return poly;
}

/** ecCodewords returns the Reed-Solomon remainder for one block. */
export function ecCodewords(data: Uint8Array, ecLength: number): Uint8Array {
  const generator = generatorPoly(ecLength);
  const remainder = new Uint8Array(ecLength);

  for (const byte of data) {
    const factor = byte ^ remainder[0];
    remainder.copyWithin(0, 1);
    remainder[remainder.length - 1] = 0;
    for (let i = 0; i < remainder.length; i++) {
      remainder[i] ^= gfMul(generator[i + 1], factor);
    }
  }
  return remainder;
}

// ---------------------------------------------------------------------------
// Bit stream
// ---------------------------------------------------------------------------

class BitBuffer {
  private bits: number[] = [];

  push(value: number, length: number) {
    for (let i = length - 1; i >= 0; i--) {
      this.bits.push((value >>> i) & 1);
    }
  }

  get length(): number {
    return this.bits.length;
  }

  toBytes(): Uint8Array {
    const bytes = new Uint8Array(Math.ceil(this.bits.length / 8));
    this.bits.forEach((bit, index) => {
      if (bit) bytes[index >>> 3] |= 0x80 >>> (index & 7);
    });
    return bytes;
  }
}

/** chooseVersion returns the smallest version that holds the payload. */
function chooseVersion(byteLength: number): VersionSpec {
  for (const spec of VERSIONS) {
    const capacityBits = dataCodewords(spec) * 8;
    if (4 + countBits(spec.version) + byteLength * 8 <= capacityBits) {
      return spec;
    }
  }
  throw new Error('payload too long for a version 10 QR code');
}

/** buildDataCodewords assembles mode, length, payload, terminator and padding. */
function buildDataCodewords(payload: Uint8Array, spec: VersionSpec): Uint8Array {
  const capacity = dataCodewords(spec);
  const buffer = new BitBuffer();

  buffer.push(0b0100, 4); // byte mode
  buffer.push(payload.length, countBits(spec.version));
  for (const byte of payload) buffer.push(byte, 8);

  // Terminator, then pad to a byte boundary, then the two alternating pad
  // codewords the specification names.
  const capacityBits = capacity * 8;
  buffer.push(0, Math.min(4, capacityBits - buffer.length));
  if (buffer.length % 8 !== 0) buffer.push(0, 8 - (buffer.length % 8));

  const codewords = new Uint8Array(capacity);
  codewords.set(buffer.toBytes());
  for (let i = buffer.length / 8; i < capacity; i++) {
    codewords[i] = i % 2 === (buffer.length / 8) % 2 ? 0xec : 0x11;
  }
  return codewords;
}

/** interleave splits data into blocks, appends error correction, and weaves
 *  the blocks together in the order the specification requires. */
function interleave(data: Uint8Array, spec: VersionSpec): Uint8Array {
  const blocks: Uint8Array[] = [];
  const ecBlocks: Uint8Array[] = [];

  let offset = 0;
  for (let i = 0; i < spec.group1Blocks; i++) {
    const block = data.slice(offset, offset + spec.group1Data);
    offset += spec.group1Data;
    blocks.push(block);
    ecBlocks.push(ecCodewords(block, spec.ecPerBlock));
  }
  for (let i = 0; i < spec.group2Blocks; i++) {
    const block = data.slice(offset, offset + spec.group2Data);
    offset += spec.group2Data;
    blocks.push(block);
    ecBlocks.push(ecCodewords(block, spec.ecPerBlock));
  }

  const result: number[] = [];
  const longest = Math.max(...blocks.map((block) => block.length));
  for (let i = 0; i < longest; i++) {
    for (const block of blocks) {
      if (i < block.length) result.push(block[i]);
    }
  }
  for (let i = 0; i < spec.ecPerBlock; i++) {
    for (const block of ecBlocks) result.push(block[i]);
  }

  return Uint8Array.from(result);
}

// ---------------------------------------------------------------------------
// Matrix construction
// ---------------------------------------------------------------------------

interface Canvas {
  size: number;
  modules: boolean[][];
  functions: boolean[][];
}

function newCanvas(version: number): Canvas {
  const size = version * 4 + 17;
  const modules = Array.from({ length: size }, () => new Array<boolean>(size).fill(false));
  const functions = Array.from({ length: size }, () => new Array<boolean>(size).fill(false));
  return { size, modules, functions };
}

function setFunction(canvas: Canvas, x: number, y: number, dark: boolean) {
  if (x < 0 || y < 0 || x >= canvas.size || y >= canvas.size) return;
  canvas.modules[y][x] = dark;
  canvas.functions[y][x] = true;
}

function drawFinder(canvas: Canvas, cx: number, cy: number) {
  for (let dy = -4; dy <= 4; dy++) {
    for (let dx = -4; dx <= 4; dx++) {
      const distance = Math.max(Math.abs(dx), Math.abs(dy));
      // The ring at distance 2 is the light gap; distance 4 is the separator.
      setFunction(canvas, cx + dx, cy + dy, distance !== 2 && distance <= 3);
    }
  }
}

function drawAlignment(canvas: Canvas, cx: number, cy: number) {
  for (let dy = -2; dy <= 2; dy++) {
    for (let dx = -2; dx <= 2; dx++) {
      setFunction(canvas, cx + dx, cy + dy, Math.max(Math.abs(dx), Math.abs(dy)) !== 1);
    }
  }
}

function drawFunctionPatterns(canvas: Canvas, version: number) {
  const size = canvas.size;

  // Timing patterns first: the finder drawing below overwrites the parts that
  // fall inside the finder areas, which is what the specification shows.
  for (let i = 0; i < size; i++) {
    setFunction(canvas, 6, i, i % 2 === 0);
    setFunction(canvas, i, 6, i % 2 === 0);
  }

  drawFinder(canvas, 3, 3);
  drawFinder(canvas, size - 4, 3);
  drawFinder(canvas, 3, size - 4);

  const positions = ALIGNMENT_POSITIONS[version];
  const last = positions.length - 1;
  positions.forEach((cy, i) => {
    positions.forEach((cx, j) => {
      // The three corners already hold finder patterns.
      if ((i === 0 && j === 0) || (i === 0 && j === last) || (i === last && j === 0)) return;
      drawAlignment(canvas, cx, cy);
    });
  });

  // Reserve the format information areas so data placement skips them. The
  // real bits are written after the mask is chosen. Index 6 is skipped: those
  // two modules belong to the timing patterns, and blanking them here would
  // break the very lines a scanner uses to find the grid.
  for (let i = 0; i < 9; i++) {
    if (i === 6) continue;
    setFunction(canvas, 8, i, false);
    setFunction(canvas, i, 8, false);
  }
  for (let i = 0; i < 8; i++) {
    setFunction(canvas, size - 1 - i, 8, false);
    setFunction(canvas, 8, size - 1 - i, false);
  }
  // The one module that is always dark.
  setFunction(canvas, 8, size - 8, true);

  if (version >= 7) drawVersionInfo(canvas, version);
}

/** drawVersionInfo writes the BCH(18,6) protected version number, which
 *  versions 7 and up carry twice. */
function drawVersionInfo(canvas: Canvas, version: number) {
  let remainder = version;
  for (let i = 0; i < 12; i++) {
    remainder = (remainder << 1) ^ ((remainder >>> 11) * 0x1f25);
  }
  const bits = (version << 12) | remainder;

  for (let i = 0; i < 18; i++) {
    const dark = ((bits >>> i) & 1) !== 0;
    const a = canvas.size - 11 + (i % 3);
    const b = Math.floor(i / 3);
    setFunction(canvas, a, b, dark);
    setFunction(canvas, b, a, dark);
  }
}

/** formatBits returns the 15 bit BCH(15,5) protected format information for
 *  error correction level L and the given mask. */
export function formatBits(mask: number): number {
  const data = (ECC_L_FORMAT_BITS << 3) | mask;
  let remainder = data;
  for (let i = 0; i < 10; i++) {
    remainder = (remainder << 1) ^ ((remainder >>> 9) * 0x537);
  }
  return ((data << 10) | remainder) ^ 0x5412;
}

function drawFormatInfo(canvas: Canvas, mask: number) {
  const bits = formatBits(mask);
  const size = canvas.size;
  const bit = (index: number) => ((bits >>> index) & 1) !== 0;

  // First copy, around the top-left finder.
  for (let i = 0; i <= 5; i++) setFunction(canvas, 8, i, bit(i));
  setFunction(canvas, 8, 7, bit(6));
  setFunction(canvas, 8, 8, bit(7));
  setFunction(canvas, 7, 8, bit(8));
  for (let i = 9; i < 15; i++) setFunction(canvas, 14 - i, 8, bit(i));

  // Second copy, split between the other two finders.
  for (let i = 0; i < 8; i++) setFunction(canvas, size - 1 - i, 8, bit(i));
  for (let i = 8; i < 15; i++) setFunction(canvas, 8, size - 15 + i, bit(i));
  setFunction(canvas, 8, size - 8, true);
}

/** placeData walks the two-module-wide zigzag from the bottom right, skipping
 *  every function module. */
function placeData(canvas: Canvas, codewords: Uint8Array) {
  const size = canvas.size;
  let bitIndex = 0;
  const totalBits = codewords.length * 8;

  for (let right = size - 1; right >= 1; right -= 2) {
    if (right === 6) right = 5; // the vertical timing pattern is not a data column
    for (let vertical = 0; vertical < size; vertical++) {
      for (let column = 0; column < 2; column++) {
        const x = right - column;
        const upward = ((right + 1) & 2) === 0;
        const y = upward ? size - 1 - vertical : vertical;
        if (canvas.functions[y][x]) continue;
        if (bitIndex < totalBits) {
          canvas.modules[y][x] = ((codewords[bitIndex >>> 3] >>> (7 - (bitIndex & 7))) & 1) !== 0;
          bitIndex++;
        }
        // Anything past the last data bit stays light: those are the
        // remainder bits, which carry no information.
      }
    }
  }
}

function maskCondition(mask: number, x: number, y: number): boolean {
  switch (mask) {
    case 0: return (x + y) % 2 === 0;
    case 1: return y % 2 === 0;
    case 2: return x % 3 === 0;
    case 3: return (x + y) % 3 === 0;
    case 4: return (Math.floor(x / 3) + Math.floor(y / 2)) % 2 === 0;
    case 5: return ((x * y) % 2) + ((x * y) % 3) === 0;
    case 6: return (((x * y) % 2) + ((x * y) % 3)) % 2 === 0;
    case 7: return (((x + y) % 2) + ((x * y) % 3)) % 2 === 0;
    default: throw new Error(`unknown mask ${mask}`);
  }
}

function applyMask(canvas: Canvas, mask: number) {
  for (let y = 0; y < canvas.size; y++) {
    for (let x = 0; x < canvas.size; x++) {
      if (canvas.functions[y][x]) continue;
      if (maskCondition(mask, x, y)) canvas.modules[y][x] = !canvas.modules[y][x];
    }
  }
}

/** penalty scores a masked matrix by the four rules in the specification. The
 *  lowest score wins, which is what keeps large blank areas and finder-like
 *  runs out of the data region. */
function penalty(canvas: Canvas): number {
  const size = canvas.size;
  const at = (x: number, y: number) => canvas.modules[y][x];
  let score = 0;

  // Rule 1: runs of five or more identical modules in a line.
  const scoreLine = (get: (i: number) => boolean) => {
    let runColor = get(0);
    let runLength = 1;
    for (let i = 1; i < size; i++) {
      const color = get(i);
      if (color === runColor) {
        runLength++;
      } else {
        if (runLength >= 5) score += runLength - 2;
        runColor = color;
        runLength = 1;
      }
    }
    if (runLength >= 5) score += runLength - 2;
  };
  for (let y = 0; y < size; y++) scoreLine((x) => at(x, y));
  for (let x = 0; x < size; x++) scoreLine((y) => at(x, y));

  // Rule 2: 2x2 blocks of one colour.
  for (let y = 0; y < size - 1; y++) {
    for (let x = 0; x < size - 1; x++) {
      const color = at(x, y);
      if (color === at(x + 1, y) && color === at(x, y + 1) && color === at(x + 1, y + 1)) {
        score += 3;
      }
    }
  }

  // Rule 3: the finder-like 1:1:3:1:1 pattern with four light modules beside it.
  const patternA = [true, false, true, true, true, false, true, false, false, false, false];
  const patternB = [false, false, false, false, true, false, true, true, true, false, true];
  const matches = (get: (i: number) => boolean, start: number, pattern: boolean[]) =>
    pattern.every((value, index) => get(start + index) === value);
  for (let y = 0; y < size; y++) {
    for (let x = 0; x + 11 <= size; x++) {
      if (matches((i) => at(i, y), x, patternA)) score += 40;
      if (matches((i) => at(i, y), x, patternB)) score += 40;
    }
  }
  for (let x = 0; x < size; x++) {
    for (let y = 0; y + 11 <= size; y++) {
      if (matches((i) => at(x, i), y, patternA)) score += 40;
      if (matches((i) => at(x, i), y, patternB)) score += 40;
    }
  }

  // Rule 4: deviation from an even balance of dark and light.
  let dark = 0;
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) if (at(x, y)) dark++;
  }
  const percentage = (dark * 100) / (size * size);
  score += Math.floor(Math.abs(percentage - 50) / 5) * 10;

  return score;
}

/**
 * encodeQR turns text into a square matrix of dark/light modules, without the
 * quiet zone. Throws only if the text does not fit a version 10 symbol.
 */
export function encodeQR(text: string): boolean[][] {
  const payload = new TextEncoder().encode(text);
  const spec = chooseVersion(payload.length);
  const codewords = interleave(buildDataCodewords(payload, spec), spec);

  let best: Canvas | null = null;
  let bestScore = Number.POSITIVE_INFINITY;

  for (let mask = 0; mask < 8; mask++) {
    const canvas = newCanvas(spec.version);
    drawFunctionPatterns(canvas, spec.version);
    placeData(canvas, codewords);
    applyMask(canvas, mask);
    drawFormatInfo(canvas, mask);

    const score = penalty(canvas);
    if (score < bestScore) {
      bestScore = score;
      best = canvas;
    }
  }

  if (!best) throw new Error('QR encoding produced no candidate');
  return best.modules;
}

/** qrPathData renders the dark modules as one SVG path, which is far lighter
 *  than a few thousand rect elements. */
export function qrPathData(modules: boolean[][], quietZone: number): string {
  const parts: string[] = [];
  modules.forEach((row, y) => {
    row.forEach((dark, x) => {
      if (dark) parts.push(`M${x + quietZone} ${y + quietZone}h1v1h-1z`);
    });
  });
  return parts.join('');
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export interface TwoFactorQRCodeProps {
  /** The otpauth:// URI to encode. */
  value: string;
  /** Rendered edge length in CSS pixels. */
  size?: number;
  /** Accessible description; the URI itself is never read out. */
  label?: string;
  className?: string;
}

/**
 * TwoFactorQRCode draws the provisioning URI as an inline SVG. The whole code
 * is built in the browser: nothing about the secret is sent anywhere to be
 * rendered.
 */
export default function TwoFactorQRCode({
  value,
  size = 220,
  label = 'Two-factor enrolment QR code',
  className,
}: TwoFactorQRCodeProps) {
  const result = React.useMemo(() => {
    try {
      const modules = encodeQR(value);
      return { modules, error: null as string | null };
    } catch (err) {
      return { modules: null, error: err instanceof Error ? err.message : 'QR encoding failed' };
    }
  }, [value]);

  if (!result.modules) {
    // Failing soft matters here: the base32 secret and the URI are both on
    // screen next to this, so enrolment still completes without the picture.
    return (
      <div
        className={`flex items-center justify-center rounded-md border border-gray-200 bg-gray-50 p-4 text-center text-xs text-gray-600 ${className ?? ''}`}
        style={{ width: size, height: size }}
      >
        This code could not be drawn. Enter the setup key below by hand instead.
      </div>
    );
  }

  const quietZone = 4;
  const extent = result.modules.length + quietZone * 2;

  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${extent} ${extent}`}
      width={size}
      height={size}
      shapeRendering="crispEdges"
      className={`rounded-md border border-gray-200 bg-white ${className ?? ''}`}
    >
      <rect width={extent} height={extent} fill="#ffffff" />
      <path d={qrPathData(result.modules, quietZone)} fill="#111827" />
    </svg>
  );
}
