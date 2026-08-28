/**
 * Turning what the file manager API sends into something an operator reads,
 * and deciding what the editor may safely open.
 *
 * The two judgement calls that live here are worth stating plainly:
 *
 *  - A file is opened in the editor only when it looks like text. The API has
 *    no "is this binary" answer, so the check is made twice on this side: once
 *    on the mime type the listing reports, and again on the bytes that come
 *    back, because .conf, .env and Dockerfile all arrive as
 *    application/octet-stream and are perfectly editable text.
 *  - Permissions are shown in both forms. The API speaks octal on the way in
 *    (POST /files/chmod takes "0644") and symbolic on the way out (the listing
 *    sends Go's "-rw-r--r--"), and an operator thinks in whichever they were
 *    taught, so neither form is hidden.
 */

import type { FileEntry } from '@/types/files';

const NUL = String.fromCharCode(0);

/** Shown in place of a value the API did not send. Never a zero. */
export const UNKNOWN = '—';

/** Bytes in a human unit. Zero is a real size and stays "0 B". */
export function formatBytes(bytes: unknown): string {
  const value = typeof bytes === 'number' ? bytes : Number(bytes);
  if (!Number.isFinite(value) || value < 0) return UNKNOWN;
  if (value === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  const scaled = value / Math.pow(1024, i);
  return `${i === 0 ? scaled.toFixed(0) : scaled.toFixed(1)} ${units[i]}`;
}

/**
 * The API formats mod_time as "2006-01-02 15:04:05" in the SERVER's local time
 * and sends no zone, so it is shown exactly as received. Re-reading it as a
 * browser-local date would silently shift every timestamp by the offset
 * between the two machines.
 */
export function formatModTime(value: string | undefined): string {
  const raw = String(value || '').trim();
  return raw || UNKNOWN;
}

/** The nine permission characters out of Go's FileMode string, or "" when absent. */
export function permissionChars(mode: string | undefined): string {
  const raw = String(mode || '');
  if (raw.length < 9) return '';
  return raw.slice(-9);
}

/** "-rw-r--r--" becomes "0644". Returns "" when the mode cannot be read. */
export function octalFromMode(mode: string | undefined): string {
  const chars = permissionChars(mode);
  if (!chars) return '';
  let out = '0';
  for (let group = 0; group < 3; group += 1) {
    const r = chars[group * 3] === 'r' ? 4 : 0;
    const w = chars[group * 3 + 1] === 'w' ? 2 : 0;
    const x = chars[group * 3 + 2] === 'x' ? 1 : 0;
    out += String(r + w + x);
  }
  return out;
}

/** "644" or "0644" becomes "rw-r--r--". Returns "" for anything else. */
export function symbolicFromOctal(octal: string): string {
  const digits = String(octal || '').trim().replace(/^0+/, '') || '0';
  const padded = digits.padStart(3, '0');
  if (!/^[0-7]{3}$/.test(padded)) return '';
  let out = '';
  for (const digit of padded) {
    const n = Number(digit);
    out += n & 4 ? 'r' : '-';
    out += n & 2 ? 'w' : '-';
    out += n & 1 ? 'x' : '-';
  }
  return out;
}

/**
 * A mode string the chmod endpoint will accept. The service refuses anything
 * outside 0000-0777 and refuses the setuid, setgid and sticky bits outright,
 * so those are refused here too rather than sent and bounced.
 */
export function validateOctalMode(value: string): string | null {
  const raw = String(value || '').trim();
  if (!raw) return 'Enter a mode, for example 644.';
  if (!/^[0-7]{3,4}$/.test(raw)) {
    return 'A mode is three or four octal digits, for example 644 or 0755.';
  }
  if (raw.length === 4 && raw[0] !== '0') {
    return 'The panel API accepts permission bits only. Setuid, setgid and sticky bits are refused by the server.';
  }
  return null;
}

/** True when the "other" group has the write bit, i.e. anyone on the box can change it. */
export function isWorldWritable(octal: string): boolean {
  const digits = String(octal || '').trim();
  if (!/^[0-7]{3,4}$/.test(digits)) return false;
  const other = Number(digits[digits.length - 1]);
  return (other & 2) === 2;
}

/** True when the mode makes a file executable by anyone other than its owner. */
export function isGroupOrWorldExecutable(octal: string): boolean {
  const digits = String(octal || '').trim();
  if (!/^[0-7]{3,4}$/.test(digits)) return false;
  const group = Number(digits[digits.length - 2]);
  const other = Number(digits[digits.length - 1]);
  return (group & 1) === 1 || (other & 1) === 1;
}

/**
 * Mirrors service.searchPatternRe exactly. Checking it here means the operator
 * is told which characters are allowed instead of receiving a 500 with
 * "search pattern contains unsupported characters".
 */
const SEARCH_PATTERN_RE = /^[A-Za-z0-9._*?[\]-]{1,128}$/;

export function validateSearchPattern(pattern: string): string | null {
  const raw = String(pattern || '').trim();
  if (!raw) return 'Enter something to search for, for example *.php.';
  if (!SEARCH_PATTERN_RE.test(raw)) {
    return 'The server accepts letters, digits and . _ - * ? [ ] only, up to 128 characters.';
  }
  return null;
}

/* -------------------------------------------------------------------------- */
/* Text or binary                                                             */
/* -------------------------------------------------------------------------- */

/** Mime types the API reports for content that is definitely not editable text. */
const BINARY_MIME_PREFIXES = ['image/', 'video/', 'audio/'];
const BINARY_MIME_TYPES = new Set([
  'application/pdf',
  'application/zip',
  'application/gzip',
  'application/x-tar',
]);

/** Extensions the API maps to application/octet-stream but which are still not text. */
const BINARY_EXTENSIONS = new Set([
  'bin', 'br', 'bz2', 'class', 'db', 'dll', 'dmg', 'exe', 'gz', 'ico', 'iso',
  'jar', 'lz', 'lzma', 'mo', 'o', 'obj', 'odt', 'ods', 'otf', 'pdf', 'pyc',
  'rar', 'so', 'sqlite', 'sqlite3', 'tar', 'tgz', 'ttf', 'woff', 'woff2',
  'xz', 'zip', 'zst', '7z',
]);

export function extensionOf(name: string): string {
  const base = String(name || '');
  const idx = base.lastIndexOf('.');
  if (idx <= 0 || idx === base.length - 1) return '';
  return base.slice(idx + 1).toLowerCase();
}

/**
 * Whether the editor should even attempt to read this entry. Answering before
 * the request avoids pulling a 9 MB JPEG through JSON only to refuse it.
 * Returns a reason when the answer is no.
 */
export function binaryReasonFromEntry(entry: FileEntry): string | null {
  const mime = String(entry.mime_type || '').toLowerCase();
  if (BINARY_MIME_PREFIXES.some((prefix) => mime.startsWith(prefix)) || BINARY_MIME_TYPES.has(mime)) {
    return `This is a ${mime} file. The editor only opens text, so opening it would show control characters and saving it would corrupt the file. Download it instead.`;
  }
  const ext = extensionOf(entry.name);
  if (ext && BINARY_EXTENSIONS.has(ext)) {
    return `A .${ext} file is binary. The editor only opens text, so opening it would show control characters and saving it would corrupt the file. Download it instead.`;
  }
  return null;
}

/**
 * The same question asked of the bytes that actually arrived. The API returns
 * file content as a JSON string, so any byte that is not valid UTF-8 has
 * already become U+FFFD; a run of those, or any NUL, means this was never text.
 * Returns a reason when the content must not be shown.
 */
export function binaryReasonFromContent(content: string): string | null {
  const text = String(content || '');
  if (!text) return null;
  if (text.includes(NUL)) {
    return 'This file contains null bytes, which means it is binary rather than text. Opening it would show control characters and saving it would corrupt the file. Download it instead.';
  }
  const sample = text.slice(0, 8192);
  let replacements = 0;
  let controls = 0;
  for (let i = 0; i < sample.length; i += 1) {
    const code = sample.charCodeAt(i);
    if (code === 0xfffd) replacements += 1;
    else if (code < 32 && code !== 9 && code !== 10 && code !== 13) controls += 1;
  }
  if ((replacements + controls) / sample.length > 0.05) {
    return 'More than a twentieth of this file is bytes that are not text, so it is binary rather than text. Opening it would show control characters and saving it would corrupt the file. Download it instead.';
  }
  return null;
}

/* -------------------------------------------------------------------------- */
/* Language                                                                    */
/* -------------------------------------------------------------------------- */

/**
 * The language the editor announces, guessed from the extension. There is no
 * syntax highlighter in this codebase, so this names the language rather than
 * colouring it: an operator who is told "nginx configuration" knows they are
 * looking at the file they meant to open.
 */
const LANGUAGE_BY_EXTENSION: Record<string, string> = {
  bash: 'Shell', c: 'C', cfg: 'Configuration', conf: 'Configuration', cnf: 'Configuration',
  cpp: 'C++', cs: 'C#', css: 'CSS', csv: 'CSV', env: 'Environment file', go: 'Go',
  h: 'C header', htaccess: 'Apache configuration', htm: 'HTML', html: 'HTML',
  ini: 'INI', java: 'Java', js: 'JavaScript', json: 'JSON', jsx: 'JavaScript (JSX)',
  key: 'Key material', log: 'Log', lua: 'Lua', md: 'Markdown', mjs: 'JavaScript',
  php: 'PHP', pl: 'Perl', py: 'Python', rb: 'Ruby', rs: 'Rust', scss: 'SCSS',
  sh: 'Shell', sql: 'SQL', svg: 'SVG', toml: 'TOML', ts: 'TypeScript',
  tsx: 'TypeScript (TSX)', txt: 'Plain text', vue: 'Vue', xml: 'XML',
  yaml: 'YAML', yml: 'YAML',
};

const LANGUAGE_BY_NAME: Record<string, string> = {
  dockerfile: 'Dockerfile',
  makefile: 'Makefile',
  '.env': 'Environment file',
  '.gitignore': 'Ignore list',
  '.htaccess': 'Apache configuration',
};

export function guessLanguage(name: string): string {
  const lower = String(name || '').toLowerCase();
  if (LANGUAGE_BY_NAME[lower]) return LANGUAGE_BY_NAME[lower];
  if (lower.startsWith('nginx') || lower.endsWith('.nginx')) return 'nginx configuration';
  const ext = extensionOf(lower);
  if (ext && LANGUAGE_BY_EXTENSION[ext]) return LANGUAGE_BY_EXTENSION[ext];
  return 'Plain text';
}
