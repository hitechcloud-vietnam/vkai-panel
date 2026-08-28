/**
 * Where an FTP account may be confined, and how to tell when a directory is a
 * bad place to confine one.
 *
 * An FTP account is only as safe as its home directory. If the home directory
 * is a system path, or the parent of another customer's site, then "confined"
 * means confined to somebody else's files. So the panel normalises a path
 * before it judges it, and judges it against the roots the API actually
 * reports for the sites on this machine.
 *
 * Two limits are stated here rather than hidden:
 *
 *   1. These functions run in the browser. They are how the interface explains
 *      itself and how it stops an operator submitting an obvious mistake. They
 *      are NOT the security boundary. The boundary is the daemon's chroot plus
 *      a server-side check that resolves symlinks, and neither exists yet.
 *   2. The panel does not expose a configured site root - aaPanel keeps one in
 *      `sites_path` and this API has no equivalent - so "the site tree" here
 *      means the set of root_dir values GET /api/v1/websites returns.
 */

/** Directories no FTP account should ever be confined to. */
const SYSTEM_ROOTS = [
  '/',
  '/bin',
  '/boot',
  '/dev',
  '/etc',
  '/lib',
  '/lib64',
  '/proc',
  '/root',
  '/run',
  '/sbin',
  '/sys',
  '/usr',
  '/var',
];

/**
 * Collapse a POSIX path to a comparable absolute form: one leading slash, no
 * repeated separators, no `.` segments, no trailing slash, and `..` resolved
 * textually. A path that climbs above the root is clamped there, which is what
 * makes `/www/wwwroot/../../etc` compare equal to `/etc` instead of slipping
 * past a naive prefix test.
 */
export function normalizePath(input: string): string {
  const raw = (input ?? '').trim();
  if (raw === '') return '';
  const isAbsolute = raw.startsWith('/');
  const out: string[] = [];
  for (const segment of raw.split('/')) {
    if (segment === '' || segment === '.') continue;
    if (segment === '..') {
      if (out.length > 0 && out[out.length - 1] !== '..') out.pop();
      else if (!isAbsolute) out.push('..');
      continue;
    }
    out.push(segment);
  }
  const joined = out.join('/');
  if (isAbsolute) return `/${joined}`;
  return joined;
}

/** True when `child` is `parent` or sits underneath it, after normalisation. */
export function isInside(parent: string, child: string): boolean {
  const p = normalizePath(parent);
  const c = normalizePath(child);
  if (p === '' || c === '') return false;
  if (p === '/') return c.startsWith('/');
  return c === p || c.startsWith(`${p}/`);
}

/** The one thing wrong with a home directory, or null when nothing is. */
export interface RootProblem {
  /** Short label for a table cell. */
  label: string;
  /** The full sentence, for a tooltip or a detail line. */
  detail: string;
  severity: 'danger' | 'warning';
}

/**
 * Judge one candidate home directory against the site roots on this machine.
 *
 * `siteRoots` is every root_dir the API reported. A candidate that contains one
 * of them - without being one of them - would hand an FTP account the whole
 * tree beneath it, which is how one customer ends up inside another's files.
 */
export function assessHomeDirectory(
  candidate: string,
  siteRoots: string[]
): RootProblem | null {
  const raw = (candidate ?? '').trim();
  if (raw === '') {
    return {
      label: 'Empty',
      detail: 'No directory is recorded, so there is nothing to confine an account to.',
      severity: 'danger',
    };
  }
  if (!raw.startsWith('/')) {
    return {
      label: 'Not absolute',
      detail: `"${raw}" is a relative path. An FTP home directory has to be an absolute path, or the effective root depends on where the daemon happened to start.`,
      severity: 'danger',
    };
  }

  const path = normalizePath(raw);

  if (raw.split('/').includes('..')) {
    return {
      label: 'Traversal',
      detail: `"${raw}" contains "..". It resolves to ${path}, which is not what the path appears to say.`,
      severity: 'danger',
    };
  }
  if (SYSTEM_ROOTS.includes(path)) {
    return {
      label: 'System directory',
      detail: `${path} is a system directory. An account confined here can read and overwrite files that have nothing to do with any website.`,
      severity: 'danger',
    };
  }

  const contained = siteRoots
    .map(normalizePath)
    .filter((root) => root !== '' && root !== path && isInside(path, root));
  if (contained.length > 0) {
    const list = contained.slice(0, 3).join(', ');
    const more = contained.length > 3 ? ` and ${contained.length - 3} more` : '';
    return {
      label: 'Contains other sites',
      detail: `${path} is above ${list}${more}. An account confined here reaches every one of those sites, not just its own.`,
      severity: 'warning',
    };
  }

  return null;
}

/**
 * Validate a home directory an operator typed, against the roots they are
 * allowed to use. Returns an error string to show, or null when the path is
 * acceptable.
 *
 * Nothing on this screen can submit such a path yet - there is no create
 * endpoint - so this is exported for the create and change-home dialogs that
 * follow the backend work, and to keep the rule written down in one place
 * rather than reinvented per dialog.
 */
export function validateHomeDirectory(
  candidate: string,
  allowedRoots: string[]
): string | null {
  const problem = assessHomeDirectory(candidate, allowedRoots);
  if (problem && problem.severity === 'danger') return problem.detail;

  const path = normalizePath(candidate);
  const roots = allowedRoots.map(normalizePath).filter((root) => root !== '');
  if (roots.length === 0) {
    return 'The panel does not know of any site root on this machine, so it cannot confirm this path is inside the site tree.';
  }
  if (!roots.some((root) => isInside(root, path))) {
    return `${path} is outside the site tree. An FTP home directory has to be one of the site roots or a directory inside one.`;
  }
  return problem ? problem.detail : null;
}
