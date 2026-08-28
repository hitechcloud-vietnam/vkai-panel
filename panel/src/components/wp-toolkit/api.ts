/**
 * Every HTTP call the WP Toolkit makes, and nothing else.
 *
 * ---------------------------------------------------------------------------
 * WHY THIS FILE IS ORGANISED THE WAY IT IS
 *
 * The Go source contains two WordPress route blocks.
 *
 *   1. internal/handler/router.go mounts /api/v1/wordpress and its plugin and
 *      theme children. Those work today. They are also RECORD-ONLY: reading
 *      internal/service/wordpress.go, POST /wordpress/:id/plugins writes a row
 *      to wordpress_plugins and returns it. It does not install anything. The
 *      log line says "WordPress plugin installed" and no plugin is installed.
 *
 *   2. internal/handler/php_wordpress_runtime.go declares the real ones -
 *      install, live plugin and theme lists, core update, search-replace,
 *      staging clone and push. They are mounted by
 *      RegisterPHPWordPressRuntimeRoutes(protected, r.phpHandler, r.wordpressHandler)
 *      and, as of writing, NOTHING CALLS THAT FUNCTION. Every route in group 2
 *      answers 404.
 *
 * So the calls are split into `records` (group 1, always usable) and `runtime`
 * (group 2, usable only once that line is added), and `probeRuntime` below
 * decides which of the two the UI is allowed to offer. A control backed by
 * group 2 is disabled, with the reason on it, until the probe says otherwise.
 * ---------------------------------------------------------------------------
 */

import { api, unwrap, unwrapList } from '@/services/api';
import { errorMessage } from '@/lib/apiError';
import type {
  CreateStagingRequest,
  CreateWordPressSiteRequest,
  InstallResult,
  InstallSiteRequest,
  LivePlugin,
  LivePluginsResponse,
  LiveTheme,
  LiveThemesResponse,
  RuntimeAvailability,
  SearchReplaceReport,
  StagingDatabaseAction,
  StagingView,
  UpdateOutcome,
  WordPressPluginRecord,
  WordPressRuntimeView,
  WordPressSite,
  WordPressThemeRecord,
} from '@/types/wordpress';

const BASE = '/api/v1/wordpress';

// ---------------------------------------------------------------------------
// The probe
// ---------------------------------------------------------------------------

/**
 * A path that exists only if the runtime routes are mounted, and that is
 * guaranteed not to do anything if they are.
 *
 * `/wordpress/<not-a-uuid>/runtime` is deliberate:
 *
 *   - mounted   -> RequirePermission runs, siteAndTenant fails to parse the id,
 *                  the handler answers 400 (or 401/403 if the caller may not
 *                  touch WordPress at all). Any of those proves the route is
 *                  there.
 *   - unmounted -> gin matches no route, no middleware runs, 404.
 *
 * A valid-looking uuid would be useless: a site that does not exist also
 * answers 404, so a real uuid cannot tell "route missing" from "row missing".
 * An id that cannot be a uuid at all can only fail one way.
 */
const PROBE_PATH = `${BASE}/probe-not-a-uuid/runtime`;

/**
 * Ask the panel whether the WP-CLI routes are reachable.
 *
 * Never throws. A rejected request is answered 'unreachable' rather than
 * 'unmounted', because "the panel did not answer" and "the panel answered that
 * this feature is not there" lead an operator to different actions.
 */
export async function probeRuntime(): Promise<RuntimeAvailability> {
  try {
    await api.get(PROBE_PATH, { timeout: 8000 });
    // A 2xx here would be surprising, but it still means the route exists.
    return 'available';
  } catch (err: unknown) {
    const status = (err as { response?: { status?: number } })?.response?.status;
    if (status === 404) return 'unmounted';
    if (typeof status === 'number') return 'available';
    return 'unreachable';
  }
}

/**
 * The sentence shown wherever a runtime-backed control had to be switched off.
 * It names the file and the missing call, because "temporarily unavailable"
 * sends nobody anywhere.
 */
export const RUNTIME_UNMOUNTED_REASON =
  'The WP-CLI routes are not mounted on this panel. They exist in ' +
  'core/internal/handler/php_wordpress_runtime.go but nothing calls ' +
  'RegisterPHPWordPressRuntimeRoutes(protected, r.phpHandler, r.wordpressHandler) ' +
  'inside the protected group in router.go, so every one of them answers 404.';

export const RUNTIME_UNREACHABLE_REASON =
  'The panel API did not answer the capability probe, so it is not known whether ' +
  'the WP-CLI routes are available. Reload once the API is reachable again.';

/** The reason string that matches an availability value, or null when fine. */
export function runtimeReason(availability: RuntimeAvailability): string | null {
  if (availability === 'unmounted') return RUNTIME_UNMOUNTED_REASON;
  if (availability === 'unreachable') return RUNTIME_UNREACHABLE_REASON;
  if (availability === 'checking') return 'Checking which WordPress operations this panel supports.';
  return null;
}

// ---------------------------------------------------------------------------
// Group 1: the record routes router.go mounts today
// ---------------------------------------------------------------------------

/**
 * Reading the record routes' responses.
 *
 * These routes do NOT use utils.Success, so they do NOT return the
 * `{ success, data }` envelope that unwrap() in services/api.ts expects, and
 * every other section of this panel relies on. internal/handler/wordpress.go
 * calls c.JSON directly:
 *
 *   GET  /wordpress            -> { sites, total, limit, offset }
 *   GET  /wordpress/server/:id -> { sites }
 *   GET  /wordpress/:id        -> the site object, bare
 *   POST /wordpress            -> the site object, bare
 *   GET  /wordpress/:id/plugins -> { plugins }
 *   GET  /wordpress/:id/themes  -> { themes }
 *   errors                      -> { error: "message" }, a string, not an object
 *
 * Meanwhile the runtime routes in php_wordpress_runtime.go DO use
 * utils.Success. Two envelopes in one feature, so both readers below accept
 * either shape: `data` is unwrapped when it is there, and the bare body is used
 * when it is not. Reading only the standard envelope is not a crash - it is a
 * permanently empty list, which is the failure that is hard to notice.
 */
function readRecordList<T>(res: unknown, key: 'sites' | 'plugins' | 'themes'): T[] {
  const body = (res as { data?: unknown })?.data;
  if (!body || typeof body !== 'object') return [];
  const envelope = body as Record<string, unknown>;
  const inner =
    'data' in envelope && envelope.data && typeof envelope.data === 'object'
      ? (envelope.data as Record<string, unknown>)
      : envelope;
  if (Array.isArray(inner)) return inner as T[];
  const named = inner[key];
  if (Array.isArray(named)) return named as T[];
  const items = inner.items;
  return Array.isArray(items) ? (items as T[]) : [];
}

function readRecordObject<T>(res: unknown): T | null {
  const body = (res as { data?: unknown })?.data;
  if (!body || typeof body !== 'object') return null;
  const envelope = body as Record<string, unknown>;
  if ('data' in envelope && envelope.data && typeof envelope.data === 'object') {
    return envelope.data as T;
  }
  return envelope as T;
}

export const records = {
  /** GET /api/v1/wordpress. Answers { sites, total, limit, offset }. */
  async list(limit = 200, offset = 0): Promise<WordPressSite[]> {
    const res = await api.get(BASE, { params: { limit, offset } });
    return readRecordList<WordPressSite>(res, 'sites');
  },

  async get(id: string): Promise<WordPressSite | null> {
    const res = await api.get(`${BASE}/${encodeURIComponent(id)}`);
    return readRecordObject<WordPressSite>(res);
  },

  /**
   * POST /api/v1/wordpress.
   *
   * This writes the panel's record of an installation. On a panel where the
   * runtime routes are mounted it is step one of two, and POST
   * /wordpress/:id/install is what actually puts WordPress on disk. On a panel
   * where they are not, this is all that can happen, and the Add WordPress
   * screen says so in as many words rather than calling itself an installer.
   */
  async create(body: CreateWordPressSiteRequest): Promise<WordPressSite | null> {
    const res = await api.post(BASE, body);
    return readRecordObject<WordPressSite>(res);
  },

  async update(id: string, body: Record<string, unknown>): Promise<WordPressSite | null> {
    const res = await api.put(`${BASE}/${encodeURIComponent(id)}`, body);
    return readRecordObject<WordPressSite>(res);
  },

  async remove(id: string): Promise<void> {
    await api.delete(`${BASE}/${encodeURIComponent(id)}`);
  },

  /**
   * GET /api/v1/wordpress/:id/plugins - the panel's stored rows.
   *
   * These carry a version but no notion of an available update, because
   * nothing ever compares them with wordpress.org. They are useful as a count
   * and misleading as anything else, so callers must not derive "out of date"
   * from them.
   */
  async plugins(siteId: string): Promise<WordPressPluginRecord[]> {
    const res = await api.get(`${BASE}/${encodeURIComponent(siteId)}/plugins`);
    return readRecordList<WordPressPluginRecord>(res, 'plugins');
  },

  async themes(siteId: string): Promise<WordPressThemeRecord[]> {
    const res = await api.get(`${BASE}/${encodeURIComponent(siteId)}/themes`);
    return readRecordList<WordPressThemeRecord>(res, 'themes');
  },
};

// ---------------------------------------------------------------------------
// Group 2: the WP-CLI routes, live only when the probe says 'available'
// ---------------------------------------------------------------------------

export const runtime = {
  /** POST /api/v1/wordpress/:id/install - download, configure, install, chown. */
  async install(siteId: string, body: InstallSiteRequest): Promise<InstallResult | null> {
    const res = await api.post(`${BASE}/${encodeURIComponent(siteId)}/install`, body, {
      // A real install downloads WordPress and runs several WP-CLI commands.
      // The 15s default would abort a request the server is still working on.
      timeout: 180000,
    });
    return unwrap<InstallResult>(res, null);
  },

  /** GET /api/v1/wordpress/:id/runtime - which unix user this site runs as. */
  async view(siteId: string): Promise<WordPressRuntimeView | null> {
    const res = await api.get(`${BASE}/${encodeURIComponent(siteId)}/runtime`);
    return unwrap<WordPressRuntimeView>(res, null);
  },

  /** PUT /api/v1/wordpress/:id/runtime. */
  async setView(
    siteId: string,
    body: { run_as_user: string; run_as_group?: string; php_version?: string },
  ): Promise<WordPressRuntimeView | null> {
    const res = await api.put(`${BASE}/${encodeURIComponent(siteId)}/runtime`, body);
    return unwrap<WordPressRuntimeView>(res, null);
  },

  /** GET /api/v1/wordpress/:id/plugins/live - what is really installed. */
  async livePlugins(siteId: string): Promise<LivePlugin[]> {
    const res = await api.get(`${BASE}/${encodeURIComponent(siteId)}/plugins/live`, {
      timeout: 45000,
    });
    const body = unwrap<LivePluginsResponse>(res, null);
    return Array.isArray(body?.plugins) ? (body as LivePluginsResponse).plugins as LivePlugin[] : [];
  },

  /** GET /api/v1/wordpress/:id/themes/live. */
  async liveThemes(siteId: string): Promise<LiveTheme[]> {
    const res = await api.get(`${BASE}/${encodeURIComponent(siteId)}/themes/live`, {
      timeout: 45000,
    });
    const body = unwrap<LiveThemesResponse>(res, null);
    return Array.isArray(body?.themes) ? (body as LiveThemesResponse).themes as LiveTheme[] : [];
  },

  /**
   * POST /api/v1/wordpress/:id/plugins/update.
   *
   * An empty `slugs` list means "update everything" on the server side, so the
   * caller must pass the list explicitly when it means a subset. Passing
   * undefined here is how you update all of them on purpose.
   */
  async updatePlugins(siteId: string, slugs?: string[]): Promise<UpdateOutcome | null> {
    const res = await api.post(
      `${BASE}/${encodeURIComponent(siteId)}/plugins/update`,
      slugs && slugs.length > 0 ? { slugs } : {},
      { timeout: 300000 },
    );
    return unwrap<UpdateOutcome>(res, null);
  },

  async updateThemes(siteId: string, slugs?: string[]): Promise<UpdateOutcome | null> {
    const res = await api.post(
      `${BASE}/${encodeURIComponent(siteId)}/themes/update`,
      slugs && slugs.length > 0 ? { slugs } : {},
      { timeout: 300000 },
    );
    return unwrap<UpdateOutcome>(res, null);
  },

  /** GET /api/v1/wordpress/:id/core/version. Returns the runtime view. */
  async coreVersion(siteId: string): Promise<WordPressRuntimeView | null> {
    const res = await api.get(`${BASE}/${encodeURIComponent(siteId)}/core/version`);
    return unwrap<WordPressRuntimeView>(res, null);
  },

  /** POST /api/v1/wordpress/:id/core/update. Omit the version for "latest". */
  async updateCore(siteId: string, version?: string): Promise<UpdateOutcome | null> {
    const res = await api.post(
      `${BASE}/${encodeURIComponent(siteId)}/core/update`,
      version ? { version } : {},
      { timeout: 300000 },
    );
    return unwrap<UpdateOutcome>(res, null);
  },

  /**
   * POST /api/v1/wordpress/:id/search-replace.
   *
   * `dry_run` is always sent. The API defaults it to true when absent, and this
   * client never relies on that default: a request that rewrites every row of a
   * customer's database should say out loud which of the two it is.
   */
  async searchReplace(
    siteId: string,
    body: { from: string; to: string; dry_run: boolean },
  ): Promise<{ report: SearchReplaceReport | null; ran_as?: string } | null> {
    const res = await api.post(`${BASE}/${encodeURIComponent(siteId)}/search-replace`, body, {
      timeout: 300000,
    });
    return unwrap<{ report: SearchReplaceReport | null; ran_as?: string }>(res, null);
  },

  /** POST /api/v1/wordpress/:id/users/password. An empty password is generated. */
  async resetPassword(
    siteId: string,
    body: { login: string; password?: string },
  ): Promise<{ login?: string; password?: string; ran_as?: string } | null> {
    const res = await api.post(`${BASE}/${encodeURIComponent(siteId)}/users/password`, body, {
      timeout: 60000,
    });
    return unwrap<{ login?: string; password?: string; ran_as?: string }>(res, null);
  },

  /** GET /api/v1/wordpress/:id/staging. 404 when this site has no staging copy. */
  async staging(siteId: string): Promise<StagingView | null> {
    const res = await api.get(`${BASE}/${encodeURIComponent(siteId)}/staging`);
    return unwrap<StagingView>(res, null);
  },

  /** POST /api/v1/wordpress/:id/staging - clone production into a staging copy. */
  async createStaging(siteId: string, body: CreateStagingRequest): Promise<StagingView | null> {
    const res = await api.post(`${BASE}/${encodeURIComponent(siteId)}/staging`, body, {
      timeout: 600000,
    });
    return unwrap<StagingView>(res, null);
  },

  /**
   * POST /api/v1/wordpress/:id/staging/push.
   *
   * `database` is a required argument of this function with no default value,
   * which is the TypeScript mirror of the server refusing an absent choice.
   * There is no overload that omits it.
   */
  async pushStaging(siteId: string, database: StagingDatabaseAction): Promise<StagingView | null> {
    const res = await api.post(
      `${BASE}/${encodeURIComponent(siteId)}/staging/push`,
      { database },
      { timeout: 600000 },
    );
    return unwrap<StagingView>(res, null);
  },

  /** DELETE /api/v1/wordpress/:id/staging - forgets the record, keeps the files. */
  async deleteStaging(siteId: string): Promise<void> {
    await api.delete(`${BASE}/${encodeURIComponent(siteId)}/staging`);
  },
};

// ---------------------------------------------------------------------------
// Supporting reads from other sections
//
// These are all routes router.go mounts today. They are here so the Add
// WordPress form can offer real servers, real databases and real PHP versions
// instead of free-text fields the operator has to get right from memory.
// ---------------------------------------------------------------------------

export interface ServerOption {
  id: string;
  name?: string;
  hostname?: string;
  ip_address?: string;
}

export interface DatabaseOption {
  id: string;
  name?: string;
  username?: string;
  database_server_id?: string;
  status?: string;
}

export interface PHPVersionOption {
  id: string;
  version?: string;
  is_default?: boolean;
  is_active?: boolean;
}

export interface WebsiteOption {
  id: string;
  domain?: string;
  root_dir?: string;
  php_version?: string;
  server_id?: string;
}

export interface BackupJobRecord {
  id: string;
  name?: string;
  type?: string;
  resource_id?: string;
  status?: string;
}

export interface BackupRunRecord {
  id: string;
  job_id?: string;
  size?: number;
  path?: string;
  status?: string;
  started_at?: string;
  completed_at?: string | null;
}

export const supporting = {
  async servers(): Promise<ServerOption[]> {
    const res = await api.get('/api/v1/servers', { params: { page: 1, per_page: 100 } });
    return unwrapList<ServerOption>(res);
  },
  /**
   * POST /api/v1/websites. Uses utils.Created, so the website is under `data` -
   * unlike the WordPress record routes two sections up, which are bare.
   */
  async createWebsite(body: {
    domain: string;
    server_id: string;
    web_server_type: string;
    root_dir?: string;
    php_version?: string;
    site_type?: string;
  }): Promise<WebsiteOption | null> {
    const res = await api.post('/api/v1/websites', body);
    return unwrap<WebsiteOption>(res, null);
  },
  async websites(): Promise<WebsiteOption[]> {
    const res = await api.get('/api/v1/websites', { params: { page: 1, per_page: 200 } });
    return unwrapList<WebsiteOption>(res);
  },
  async databases(): Promise<DatabaseOption[]> {
    const res = await api.get('/api/v1/databases');
    return unwrapList<DatabaseOption>(res);
  },
  async phpVersions(): Promise<PHPVersionOption[]> {
    const res = await api.get('/api/v1/php/versions');
    return unwrapList<PHPVersionOption>(res);
  },
  /** Backup jobs. Used to find the one whose resource_id is a site's website_id. */
  async backupJobs(): Promise<BackupJobRecord[]> {
    const res = await api.get('/api/v1/backups/jobs');
    return unwrapList<BackupJobRecord>(res);
  },
  /** The 50 most recent backup runs for this tenant. The API caps it, not us. */
  async backupRecords(): Promise<BackupRunRecord[]> {
    const res = await api.get('/api/v1/backups/records');
    return unwrapList<BackupRunRecord>(res);
  },
};

/** Re-exported so screens do not each import from two places. */
export { errorMessage };
