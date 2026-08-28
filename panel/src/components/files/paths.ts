/**
 * Path arithmetic for the file manager.
 *
 * The API is the security boundary - service.ResolvePath re-anchors everything
 * under the jail root and refuses what escapes it. Nothing here pretends
 * otherwise. These helpers exist so the interface shows the operator a path
 * that matches what the server will act on, and so a stray "//" or trailing
 * slash never turns into a second request for the same directory.
 */

/** A NUL byte, which the API rejects outright (service.ContainsNullByte). */
const NUL = String.fromCharCode(0);

/** Collapse repeated separators and drop a trailing slash. "" becomes "/". */
export function normalizePath(input: string): string {
  const raw = String(input || '').trim();
  if (!raw) return '/';
  const collapsed = raw.replace(/\/+/g, '/');
  if (collapsed === '/') return '/';
  return collapsed.replace(/\/+$/, '') || '/';
}

/** The last segment of a path. "/a/b" gives "b"; "/" gives "/". */
export function baseName(path: string): string {
  const p = normalizePath(path);
  if (p === '/') return '/';
  const idx = p.lastIndexOf('/');
  return idx === -1 ? p : p.slice(idx + 1);
}

/** The containing directory. "/a/b" gives "/a"; "/a" and "/" give "/". */
export function dirName(path: string): string {
  const p = normalizePath(path);
  if (p === '/') return '/';
  const idx = p.lastIndexOf('/');
  if (idx <= 0) return '/';
  return p.slice(0, idx);
}

/** Join a directory and a name into one absolute path. */
export function joinPath(dir: string, name: string): string {
  const d = normalizePath(dir);
  const n = String(name || '').replace(/^\/+/, '');
  if (!n) return d;
  return normalizePath(d === '/' ? `/${n}` : `${d}/${n}`);
}

/** True when `path` is the root or sits inside it. */
export function isInside(root: string, path: string): boolean {
  const r = normalizePath(root);
  const p = normalizePath(path);
  if (r === '/') return true;
  return p === r || p.startsWith(`${r}/`);
}

/**
 * The parent of `path`, never above `root`. Standing at the root, the parent is
 * the root: there is no "up" out of the jail, and offering one would only
 * produce a refusal from the API.
 */
export function parentWithin(root: string, path: string): string {
  const r = normalizePath(root);
  const p = normalizePath(path);
  if (p === r || !isInside(r, p)) return r;
  const parent = dirName(p);
  return isInside(r, parent) ? parent : r;
}

export interface PathCrumb {
  label: string;
  path: string;
}

/**
 * Breadcrumb segments from the root to `path`. The first crumb is the root
 * itself, labelled by the caller, so the operator can always see where the
 * bottom of the tree is.
 */
export function crumbsWithin(root: string, path: string, rootLabel: string): PathCrumb[] {
  const r = normalizePath(root);
  const p = normalizePath(path);
  const crumbs: PathCrumb[] = [{ label: rootLabel, path: r }];
  if (!isInside(r, p) || p === r) return crumbs;

  const rest = r === '/' ? p.slice(1) : p.slice(r.length + 1);
  let current = r;
  for (const segment of rest.split('/')) {
    if (!segment) continue;
    current = joinPath(current, segment);
    crumbs.push({ label: segment, path: current });
  }
  return crumbs;
}

/**
 * A file name that is safe to send as a single path segment. The API resolves
 * "../" away rather than failing loudly, so a name containing a separator would
 * silently land somewhere else; refusing it here keeps the result predictable.
 * Returns an error message, or null when the name is usable.
 */
export function validateName(name: string): string | null {
  const value = String(name || '').trim();
  if (!value) return 'Enter a name.';
  if (value === '.' || value === '..') return 'A name cannot be "." or "..".';
  if (value.includes('/')) {
    return 'A name cannot contain "/". Use Copy or Move to put it in another directory.';
  }
  if (value.includes(NUL)) return 'A name cannot contain a null byte.';
  if (value.length > 255) return 'A name can be at most 255 characters.';
  return null;
}

/** An absolute destination directory the operator typed. Returns an error, or null. */
export function validateDirectoryPath(path: string): string | null {
  const value = String(path || '').trim();
  if (!value) return 'Enter a destination directory.';
  if (!value.startsWith('/')) return 'Enter an absolute path, starting with "/".';
  if (value.includes(NUL)) return 'A path cannot contain a null byte.';
  return null;
}
