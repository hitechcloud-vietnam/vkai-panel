/**
 * The shapes the Websites screen reads, one per backend resource it touches.
 *
 * Every field name here was taken from the Go model behind the route that
 * serves it, not from what the screen wished existed:
 *
 *   ManagedWebsite      core/internal/models/models.go   type Website
 *   NodeApp             core/internal/models/nodeapp.go  type NodeApp
 *   ReverseProxy        core/internal/models/reverseproxy.go
 *   GitDeployment       core/internal/models/gitdeployment.go
 *   WordPressSite       core/internal/models/wordpress.go
 *   PHPPool             core/internal/models/php.go
 *   SSLCertificate      core/internal/models/models.go
 *
 * Where a field the UI would like does NOT exist in the model, it is absent
 * here too and named in PROJECT_TYPE_GAPS instead. A type that invents a field
 * is how a form ends up posting something the API drops on the floor.
 */

/** One Node.js project, from GET /api/v1/node-apps (paginated). */
export interface NodeApp {
  id: string;
  tenant_id?: string;
  server_id?: string;
  /** Set when the project was attached to a website row. */
  website_id?: string | null;
  name?: string;
  description?: string;
  /** Working directory the service runs in. The backend has no separate entry-file field. */
  path?: string;
  port?: number;
  node_version?: string;
  npm_version?: string;
  start_script?: string;
  stop_script?: string;
  restart_script?: string;
  env_file?: string;
  log_file?: string;
  pid_file?: string;
  status?: string;
  is_active?: boolean;
  auto_restart?: boolean;
  max_restarts?: number;
  created_at?: string;
  updated_at?: string;
}

/** Body of POST /api/v1/node-apps (models.CreateNodeAppRequest). */
export interface CreateNodeAppRequest {
  server_id: string;
  website_id?: string;
  name: string;
  description?: string;
  path: string;
  port: number;
  node_version?: string;
  start_script?: string;
  stop_script?: string;
  restart_script?: string;
  auto_restart?: boolean;
  max_restarts?: number;
}

/** One environment variable of a Node.js project, from GET /api/v1/node-apps/:id/environments. */
export interface NodeAppEnvironment {
  id: string;
  app_id?: string;
  key?: string;
  value?: string;
  is_secret?: boolean;
  created_at?: string;
  updated_at?: string;
}

/** One proxy project, from GET /api/v1/reverse-proxy. */
export interface ReverseProxy {
  id: string;
  tenant_id?: string;
  server_id?: string;
  website_id?: string | null;
  name?: string;
  domain?: string;
  listen_port?: number;
  target_url?: string;
  target_host?: string;
  target_port?: number;
  protocol?: string;
  ssl_enabled?: boolean;
  ssl_redirect?: boolean;
  ssl_cert_path?: string;
  ssl_key_path?: string;
  /** Extra request headers written to the upstream. The host header lives here under "Host". */
  headers?: Record<string, string> | null;
  websocket?: boolean;
  load_balancer?: boolean;
  backend_servers?: Record<string, string> | null;
  health_check?: string;
  health_interval?: number;
  status?: string;
  is_active?: boolean;
  created_at?: string;
  updated_at?: string;
}

/** Body of POST /api/v1/reverse-proxy (models.CreateReverseProxyRequest). */
export interface CreateReverseProxyRequest {
  server_id: string;
  website_id?: string;
  name: string;
  domain: string;
  listen_port?: number;
  target_url?: string;
  target_host: string;
  target_port: number;
  protocol?: string;
  ssl_enabled?: boolean;
  ssl_redirect?: boolean;
  headers?: Record<string, string>;
  websocket?: boolean;
  health_check?: string;
  health_interval?: number;
}

/** One access-log line of a proxy, from GET /api/v1/reverse-proxy/:id/access-logs. */
export interface ReverseProxyAccessLog {
  id: string;
  remote_addr?: string;
  method?: string;
  request_uri?: string;
  status?: number;
  body_bytes?: number;
  response_time?: number;
  created_at?: string;
}

/** One git deployment, from GET /api/v1/git-deployments. Carries the site's last deploy. */
export interface GitDeployment {
  id: string;
  server_id?: string;
  website_id?: string | null;
  name?: string;
  repository_url?: string;
  branch?: string;
  deploy_path?: string;
  auto_deploy?: boolean;
  status?: string;
  is_active?: boolean;
  last_deploy_at?: string | null;
  last_commit_hash?: string;
  created_at?: string;
  updated_at?: string;
}

/** One WordPress record, from GET /api/v1/wordpress. */
export interface WordPressSite {
  id: string;
  server_id?: string;
  website_id?: string | null;
  name?: string;
  domain?: string;
  path?: string;
  version?: string;
  status?: string;
  is_active?: boolean;
  auto_update?: boolean;
  last_update_at?: string | null;
}

/** One PHP-FPM pool, from GET /api/v1/php/pools?website_id=... */
export interface PHPPool {
  id: string;
  name?: string;
  php_version_id?: string;
  user?: string;
  group?: string;
  listen?: string;
  pm?: string;
  pm_max_children?: number;
  pm_start_servers?: number;
  pm_min_spare_servers?: number;
  pm_max_spare_servers?: number;
  pm_max_requests?: number;
  pm_process_idle_timeout?: string;
  is_active?: boolean;
  website_id?: string;
  server_id?: string;
}

/** Body of PUT /api/v1/php/pools/:id. Every field is optional; the API patches. */
export interface UpdatePHPPoolRequest {
  pm?: string;
  pm_max_children?: number;
  pm_start_servers?: number;
  pm_min_spare_servers?: number;
  pm_max_spare_servers?: number;
  pm_max_requests?: number;
  pm_process_idle_timeout?: string;
}

/** The disk figure GET /api/v1/files/disk-usage returns for one path. */
export interface DiskUsage {
  /** Human-readable size, straight from `du -sh`. The API does not return bytes. */
  size?: string;
  filesystem?: string;
}
