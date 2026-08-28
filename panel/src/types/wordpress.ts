/**
 * Types for the WP Toolkit section.
 *
 * Every shape here mirrors something the Go API actually returns. Where a
 * field is absent from the API it is absent from the type, so a component
 * cannot render a figure the backend never sends. The one thing this file adds
 * on top of the wire shapes is `RuntimeAvailability`, which records whether the
 * WP-CLI half of the API is reachable at all - see components/wp-toolkit/api.ts.
 */

// ---------------------------------------------------------------------------
// Records the panel keeps in its own database
//
// These come from the /api/v1/wordpress routes that router.go mounts today.
// They are the panel's RECORD of an installation, not a reading taken from the
// installation. A record can be perfectly accurate and still be six months out
// of date, which is why the live shapes below exist separately and are never
// merged into these.
// ---------------------------------------------------------------------------

/** One row of GET /api/v1/wordpress. Mirrors models.WordPressSite. */
export interface WordPressSite {
  id: string;
  tenant_id?: string;
  server_id?: string;
  website_id?: string | null;
  name?: string;
  domain?: string;
  path?: string;
  db_name?: string;
  db_user?: string;
  db_host?: string;
  db_prefix?: string;
  admin_user?: string;
  admin_email?: string;
  version?: string;
  status?: string;
  is_active?: boolean;
  auto_update?: boolean;
  last_update_at?: string | null;
  created_at?: string;
  updated_at?: string;
}

/** One row of GET /api/v1/wordpress/:id/plugins. The panel's record only. */
export interface WordPressPluginRecord {
  id: string;
  site_id: string;
  name?: string;
  slug?: string;
  version?: string;
  status?: string;
  is_active?: boolean;
  auto_update?: boolean;
}

/** One row of GET /api/v1/wordpress/:id/themes. The panel's record only. */
export interface WordPressThemeRecord {
  id: string;
  site_id: string;
  name?: string;
  slug?: string;
  version?: string;
  is_active?: boolean;
  auto_update?: boolean;
}

/** Body of POST /api/v1/wordpress. Every required field is required here too. */
export interface CreateWordPressSiteRequest {
  server_id: string;
  website_id?: string;
  name: string;
  domain: string;
  path: string;
  db_name: string;
  db_user: string;
  db_password: string;
  db_host?: string;
  db_prefix?: string;
  admin_user: string;
  admin_password: string;
  admin_email: string;
  auto_update?: boolean;
}

// ---------------------------------------------------------------------------
// Live readings, taken through WP-CLI
//
// Everything below is served by the runtime routes in
// core/internal/handler/php_wordpress_runtime.go. Those routes are mounted by
// RegisterPHPWordPressRuntimeRoutes, and whether that function is called at
// start-up is a fact this UI has to discover at run time rather than assume.
// ---------------------------------------------------------------------------

/** GET /api/v1/wordpress/:id/runtime and /core/version. Mirrors models.RuntimeView. */
export interface WordPressRuntimeView {
  run_as_user?: string;
  run_as_group?: string;
  php_version?: string;
  installed_wordpress_version?: string;
  last_ran_as?: string;
  last_command?: string;
  last_ran_at?: string;
}

/**
 * One row of `wp plugin list --format=json`, as returned under `plugins` by
 * GET /api/v1/wordpress/:id/plugins/live. Mirrors wpcli.Plugin.
 *
 * `update` is WP-CLI's own vocabulary: "available", "none", "unavailable".
 * Only "available" means an update exists; "unavailable" means WP-CLI could not
 * find out, and counting it as up to date would be a lie.
 */
export interface LivePlugin {
  name: string;
  status?: string;
  update?: string;
  version?: string;
  update_version?: string;
  auto_update?: string;
}

/** One row of `wp theme list --format=json`. Mirrors wpcli.Theme. */
export interface LiveTheme {
  name: string;
  status?: string;
  update?: string;
  version?: string;
  update_version?: string;
}

/** Envelope of GET /api/v1/wordpress/:id/plugins/live. */
export interface LivePluginsResponse {
  plugins: LivePlugin[] | null;
  ran_as?: string;
}

/** Envelope of GET /api/v1/wordpress/:id/themes/live. */
export interface LiveThemesResponse {
  themes: LiveTheme[] | null;
  ran_as?: string;
}

/** Result of the three update endpoints. Mirrors service.UpdateOutcome. */
export interface UpdateOutcome {
  ran_as?: string;
  output?: string;
  requested?: string[] | null;
}

/** Result of POST /api/v1/wordpress/:id/install. Mirrors service.InstallResult. */
export interface InstallResult {
  site_id: string;
  path?: string;
  url?: string;
  installed_version?: string;
  ran_as?: string;
  ownership?: string;
  admin_user?: string;
}

/** Body of POST /api/v1/wordpress/:id/install. */
export interface InstallSiteRequest {
  run_as_user: string;
  run_as_group?: string;
  site_title?: string;
  core_version?: string;
  locale?: string;
}

/** Mirrors wpcli.SearchReplaceReport, returned under `report`. */
export interface SearchReplaceReport {
  from?: string;
  to?: string;
  dry_run?: boolean;
  tables?: number;
  rows?: number;
  output?: string;
  [key: string]: unknown;
}

// ---------------------------------------------------------------------------
// Staging - the Data Copy screen
// ---------------------------------------------------------------------------

/**
 * What a push does to the production database.
 *
 * These three strings are the whole safety contract of the Data Copy screen and
 * they are copied verbatim from wpcli.DatabaseAction. There is deliberately no
 * fourth member and no default: the API rejects an absent choice with
 * ErrDatabaseChoiceRequired, and this UI must never send one it invented.
 */
export type StagingDatabaseAction =
  | 'keep_production'
  | 'overwrite_production'
  | 'database_only';

/** Mirrors wpcli.CloneResult. */
export interface StagingCloneResult {
  staging_dir?: string;
  staging_url?: string;
  url_rewrite?: SearchReplaceReport | null;
  ran_as?: string;
}

/** Mirrors wpcli.PushResult. The two backup paths are the way back. */
export interface StagingPushResult {
  database_action?: StagingDatabaseAction | '';
  files_copied?: boolean;
  database_copied?: boolean;
  backup_path?: string;
  database_backup_path?: string;
  url_rewrite?: SearchReplaceReport | null;
  ran_as?: string;
}

/** Mirrors models.StagingHistory. Every field is absent until it has happened. */
export interface StagingHistory {
  last_clone_at?: string;
  last_push_at?: string;
  last_push_database?: string;
  last_push_files_backup?: string;
  last_push_database_backup?: string;
  last_error?: string;
}

/** Mirrors service.StagingView, the body of every /staging response. */
export interface StagingView {
  id: string;
  staging_domain?: string;
  staging_path?: string;
  staging_url?: string;
  status?: string;
  ran_as?: string;
  history?: StagingHistory;
  clone?: StagingCloneResult | null;
  push?: StagingPushResult | null;
}

/** Body of POST /api/v1/wordpress/:id/staging. */
export interface CreateStagingRequest {
  subdomain?: string;
  db_name: string;
  db_user: string;
  db_password: string;
  db_host?: string;
  block_indexing?: boolean;
}

// ---------------------------------------------------------------------------
// Runtime availability
// ---------------------------------------------------------------------------

/**
 * Whether the WP-CLI half of the API answers at all.
 *
 * 'checking'    - the probe has not come back yet.
 * 'available'   - the runtime routes are mounted. Live readings and one-click
 *                 updates are real.
 * 'unmounted'   - the routes 404. They exist in the Go source but nothing
 *                 called RegisterPHPWordPressRuntimeRoutes, so every live
 *                 control on this section would be a button that does nothing.
 * 'unreachable' - the probe itself failed (network, panel down). Different from
 *                 'unmounted', because retrying is worth it.
 */
export type RuntimeAvailability = 'checking' | 'available' | 'unmounted' | 'unreachable';

/**
 * One installation with whatever live readings could be taken for it.
 *
 * Every live field is `null` when it could not be read, never 0 and never "".
 * A site with no plugins and a site whose plugin list could not be read are
 * different facts, and an operator who cannot tell them apart cannot act on
 * either.
 */
export interface InstallationSummary {
  site: WordPressSite;
  runtime: WordPressRuntimeView | null;
  plugins: LivePlugin[] | null;
  themes: LiveTheme[] | null;
  /** Why the live readings are missing, when they are. */
  liveError: string | null;
}

/** Counts derived from a live list. Null throughout when the list is null. */
export interface UpdateCounts {
  total: number;
  outdated: number;
  /** Items WP-CLI could not check. Reported separately, never folded into `outdated`. */
  unknown: number;
}
