/**
 * The managed node, as the panel receives it.
 *
 * Field names are taken from the Go model behind GET /api/v1/servers
 * (core/internal/models/models.go, type Server) and from the servers table
 * itself. Two things are worth knowing before reading anything below.
 *
 * There is NO `name` column and no `name` field. A node is identified by its
 * hostname; use `serverLabel()` from '@/lib/servers' rather than reaching for a
 * display name that does not exist.
 *
 * The panel host is a managed node like any other: the installer registers the
 * machine it installed on as a row in this table, marked by `role`. Everything
 * that needs "the machine you are looking at" reads it from this list - there
 * is no separate local-node endpoint.
 */

/** A node's `role`, as the panel writes and reads it. `role` is a free-form varchar. */
export const LOCAL_NODE_ROLE = 'local';

/**
 * Values of `role` the panel accepts as meaning "this is the machine the panel
 * is installed on". LOCAL_NODE_ROLE is the one the panel writes; the rest are
 * tolerated so a node registered by an older installer still reads correctly.
 */
export const LOCAL_NODE_ROLE_ALIASES: readonly string[] = [
  LOCAL_NODE_ROLE,
  'localhost',
  'panel',
  'panel-host',
  'self',
];

/** One managed node. Only `id` is guaranteed; the rest may be absent while a node is still registering. */
export interface ManagedServer {
  id: string;
  tenant_id?: string;
  hostname?: string;
  ip_address?: string;
  ipv6_address?: string;
  ssh_port?: number;
  agent_status?: string;
  os?: string;
  kernel?: string;
  cpu_cores?: number;
  ram_total?: number;
  disk_total?: number;
  location?: string;
  tags?: string[] | null;
  /** 'local' on the machine the panel runs on. See LOCAL_NODE_ROLE_ALIASES. */
  role?: string;
  /** The web server a new site on this node uses unless told otherwise. */
  web_server_type?: string;
  status?: string;
  last_seen_at?: string | null;
  created_at?: string;
  updated_at?: string;

  /*
   * Fields below are expected from the local-node work but may not be sent
   * yet. Every reader treats them as optional and falls back rather than
   * showing a zero.
   */

  /**
   * Every web server installed on this node, so one machine can run more than
   * one - the aaPanel arrangement. When absent the panel offers the full list
   * of supported web servers instead of guessing.
   */
  web_servers?: string[] | null;
  /** Version of the agent running on this node. */
  agent_version?: string;
}

/**
 * The latest sample for one node, from GET /api/v1/servers/:id/metrics.
 * Mirrors models.ServerMetric. Absent when no agent has reported yet, which is
 * shown as unavailable - never as zero.
 */
export interface ServerMetrics {
  server_id?: string;
  cpu_percent?: number;
  ram_used?: number;
  ram_total?: number;
  disk_used?: number;
  disk_total?: number;
  net_in?: number;
  net_out?: number;
  load1?: number;
  load5?: number;
  load15?: number;
  timestamp?: string;
}

/** Web servers the API accepts for a website (CreateWebsiteRequest.web_server_type). */
export const WEB_SERVER_TYPES = [
  'nginx',
  'apache',
  'caddy',
  'litespeed',
  'openlitespeed',
  'traefik',
] as const;

export type WebServerType = (typeof WEB_SERVER_TYPES)[number];

/** One website, from GET /api/v1/websites. Mirrors models.Website. */
export interface ManagedWebsite {
  id: string;
  tenant_id?: string;
  server_id?: string;
  domain?: string;
  root_dir?: string;
  web_server_type?: string;
  php_version?: string;
  site_type?: string;
  status?: string;
  ssl_enabled?: boolean;
  created_at?: string;
  updated_at?: string;
}

/** Body of POST /api/v1/websites (models.CreateWebsiteRequest). */
export interface CreateWebsiteRequest {
  domain: string;
  server_id: string;
  web_server_type: string;
  root_dir?: string;
  php_version?: string;
  site_type?: string;
}

/** One certificate, from GET /api/v1/ssl. Mirrors models.SSLCertificate. */
export interface SSLCertificate {
  id: string;
  tenant_id?: string;
  website_id?: string | null;
  domain?: string;
  issuer?: string;
  not_before?: string;
  not_after?: string;
  status?: string;
  auto_renew?: boolean;
  /** 'letsencrypt' or 'custom'. */
  source?: string;
  created_at?: string;
  updated_at?: string;
}

/**
 * Body of POST /api/v1/ssl/letsencrypt.
 *
 * `domain` and `webroot` are what the handler binds today. `server_id` is sent
 * whenever the operator has picked a node, so that a panel managing more than
 * one machine issues on the machine the operator chose rather than on whichever
 * one the backend assumes.
 */
export interface IssueLetsEncryptRequest {
  domain: string;
  webroot: string;
  server_id?: string;
}
