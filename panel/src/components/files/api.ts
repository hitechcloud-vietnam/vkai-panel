'use client';

/**
 * The twelve file manager endpoints, and nothing else.
 *
 * Each function below maps onto a route that is mounted in
 * core/internal/handler/router.go under /api/v1/files, behind
 * RequirePermission("website"). Anything a file manager usually has but this
 * API does not - changing an owner, compressing a directory, deleting a batch
 * in one request - is deliberately absent rather than faked, because a control
 * that looks real and does nothing is worse than a missing one.
 *
 * Two details are not obvious from the route list and are handled here:
 *
 *  - The shared axios instance has a 15 second timeout, which is fine for a
 *    directory listing and fatal for a 100 MB upload. Upload and download pass
 *    timeout: 0 and carry their own AbortSignal instead.
 *  - That instance also defaults Content-Type to application/json, and axios
 *    turns FormData into JSON when it sees that header. The upload therefore
 *    sets multipart/form-data explicitly; the browser fills in the boundary.
 */

import { api, unwrap } from '@/services/api';
import { errorMessage } from '@/lib/apiError';
import type { DiskUsage, FileContent, FileEntry } from '@/types/files';

const BASE = '/api/v1/files';

/** The API's own cap: service.MaxUploadBytes, 100 MiB per file. */
export const MAX_UPLOAD_BYTES = 100 * 1024 * 1024;

/** The API refuses to read anything larger; service.ReadFile checks this first. */
export const MAX_READ_BYTES = 10 * 1024 * 1024;

/**
 * Extensions POST /files/upload refuses outright (service.blockedUploadExts).
 * Every extension in a name is checked by the server, so "shell.php.txt" is
 * refused as well. Listing them here lets the panel say so before a 100 MB
 * body is sent and thrown away.
 */
export const BLOCKED_UPLOAD_EXTENSIONS = [
  'php', 'php3', 'php4', 'php5', 'php7', 'php8', 'phps', 'phtml', 'pht',
  'cgi', 'pl', 'py', 'sh', 'bash', 'jsp', 'jspx', 'asp', 'aspx', 'exe',
  'so', 'htaccess', 'htpasswd',
];

/** The reason the upload endpoint would refuse this name, or null. */
export function blockedUploadReason(name: string): string | null {
  const lower = String(name || '').toLowerCase();
  if (!lower || lower === '.' || lower === '..') return 'That is not a usable file name.';
  if (BLOCKED_UPLOAD_EXTENSIONS.includes(lower)) {
    return `The server refuses to upload a file named "${name}".`;
  }
  const parts = lower.split('.').slice(1);
  for (const part of parts) {
    if (BLOCKED_UPLOAD_EXTENSIONS.includes(part)) {
      return `The server refuses uploads with a .${part} part in the name, because the panel writes into directories a web server executes. Create the file with the editor instead.`;
    }
  }
  return null;
}

/**
 * A file manager failure, said in words.
 *
 * handler.respondFileError collapses every jail refusal - outside the root, a
 * denied system directory, a path with a null byte - into the single message
 * "Invalid path". That is correct of the API and useless to an operator, so
 * that one message is expanded here. Every other message is the server's own
 * and is passed through untouched.
 */
export function fileErrorMessage(err: unknown, fallback: string): string {
  const message = errorMessage(err, fallback);
  if (message === 'Invalid path') {
    return 'The server refused that path: it is outside the file manager root, or inside a directory the panel never opens (/etc, /root, /proc, /sys, /dev, /boot and the panel’s own directories).';
  }
  return message;
}

export const filesApi = {
  /** GET /files/list. Returns [] for an empty directory: Go sends a nil slice as null. */
  async list(path: string): Promise<FileEntry[]> {
    const res = await api.get(`${BASE}/list`, { params: { path } });
    const data = unwrap<FileEntry[]>(res, null);
    return Array.isArray(data) ? data : [];
  },

  /** GET /files/read. The server refuses anything over 10 MB. */
  async read(path: string): Promise<FileContent> {
    const res = await api.get(`${BASE}/read`, { params: { path } });
    const data = unwrap<FileContent>(res, null);
    return {
      path: String(data?.path || path),
      content: typeof data?.content === 'string' ? data.content : '',
      size: typeof data?.size === 'number' ? data.size : 0,
    };
  },

  /**
   * POST /files/write. `content` is bound with binding:"required", so the API
   * rejects an empty string - a file cannot be created or saved empty through
   * this endpoint. Callers check for that and say so rather than sending it.
   */
  async write(path: string, content: string): Promise<void> {
    await api.post(`${BASE}/write`, { path, content });
  },

  /** POST /files/mkdir. Creates parents too: the service calls os.MkdirAll. */
  async mkdir(path: string): Promise<void> {
    await api.post(`${BASE}/mkdir`, { path });
  },

  /**
   * POST /files/delete. One path per request - there is no batch route - and
   * a directory goes recursively, because the service calls os.RemoveAll.
   */
  async remove(path: string): Promise<void> {
    await api.post(`${BASE}/delete`, { path });
  },

  /** POST /files/rename. This is also the move: os.Rename does both. */
  async rename(oldPath: string, newPath: string): Promise<void> {
    await api.post(`${BASE}/rename`, { old_path: oldPath, new_path: newPath });
  },

  /** POST /files/copy. Directories are copied recursively by the service. */
  async copy(src: string, dst: string): Promise<void> {
    await api.post(`${BASE}/copy`, { src, dst });
  },

  /** POST /files/chmod. `mode` is octal text; the service allows 0000-0777 only. */
  async chmod(path: string, mode: string): Promise<void> {
    await api.post(`${BASE}/chmod`, { path, mode });
  },

  /**
   * GET /files/search. `find -maxdepth 5 -name <pattern>` under `path`. The
   * rows it returns have no owner and no mime type: the service fills those in
   * only for a listing.
   */
  async search(path: string, pattern: string): Promise<FileEntry[]> {
    const res = await api.get(`${BASE}/search`, { params: { path, pattern } });
    const data = unwrap<FileEntry[]>(res, null);
    return Array.isArray(data) ? data : [];
  },

  /** GET /files/disk-usage. `du -sh` plus the raw text of `df -h`. */
  async diskUsage(path: string): Promise<DiskUsage> {
    const res = await api.get(`${BASE}/disk-usage`, { params: { path } });
    const data = unwrap<DiskUsage>(res, null);
    return {
      size: typeof data?.size === 'string' ? data.size : undefined,
      filesystem: typeof data?.filesystem === 'string' ? data.filesystem : undefined,
    };
  },

  /**
   * GET /files/download. The route needs the Authorization header, so this
   * cannot be a plain link: the response is pulled as a blob and handed to the
   * browser from memory.
   */
  async download(path: string, signal?: AbortSignal): Promise<Blob> {
    const res = await api.get(`${BASE}/download`, {
      params: { path },
      responseType: 'blob',
      timeout: 0,
      signal,
    });
    return res.data as Blob;
  },

  /**
   * POST /files/upload. `path` is the full destination path INCLUDING the file
   * name - the service takes filepath.Base of it to validate the name and then
   * opens exactly that path for writing.
   */
  async upload(
    destination: string,
    file: File,
    options: { signal?: AbortSignal; onProgress?: (percent: number | null) => void } = {}
  ): Promise<void> {
    const form = new FormData();
    form.append('path', destination);
    form.append('file', file);
    await api.post(`${BASE}/upload`, form, {
      timeout: 0,
      signal: options.signal,
      headers: { 'Content-Type': 'multipart/form-data' },
      onUploadProgress: (event) => {
        if (!options.onProgress) return;
        const total = event.total;
        if (!total || !Number.isFinite(total)) {
          options.onProgress(null);
          return;
        }
        options.onProgress(Math.min(100, Math.round((event.loaded / total) * 100)));
      },
    });
  },
};

export default filesApi;
