/**
 * Every endpoint the Websites screen calls, and nothing it cannot call.
 *
 * This module lives beside the screen rather than in services/api.ts on
 * purpose: the shared client is edited by several people at once, and the
 * routes below are used by this screen only. It shares the same axios instance,
 * so the auth token, the refresh interceptor and the base URL are the shared
 * ones.
 *
 * EVERY route here was checked against core/internal/handler/router.go before
 * it was written down. Routes the backend defines but does not mount are listed
 * at the bottom as UNMOUNTED_ROUTES and are never called - a request to one of
 * them would 404, which is exactly the failure this screen exists not to repeat.
 */

import { api, unwrap, unwrapList } from '@/services/api';
import type {
  CreateNodeAppRequest,
  CreateReverseProxyRequest,
  DiskUsage,
  GitDeployment,
  NodeApp,
  NodeAppEnvironment,
  PHPPool,
  ReverseProxy,
  ReverseProxyAccessLog,
  UpdatePHPPoolRequest,
  WordPressSite,
} from '@/types/website';

/* ----------------------------------------------------------------------
 * Node.js projects - /api/v1/node-apps (permission: nodeapp)
 * -------------------------------------------------------------------- */

export const nodeAppApi = {
  list: async (): Promise<NodeApp[]> =>
    unwrapList<NodeApp>(await api.get('/api/v1/node-apps', { params: { page: 1, per_page: 100 } })),
  create: (data: CreateNodeAppRequest) => api.post('/api/v1/node-apps', data),
  update: (id: string, data: Record<string, unknown>) => api.put(`/api/v1/node-apps/${id}`, data),
  remove: (id: string) => api.delete(`/api/v1/node-apps/${id}`),
  start: (id: string) => api.post(`/api/v1/node-apps/${id}/start`),
  stop: (id: string) => api.post(`/api/v1/node-apps/${id}/stop`),
  restart: (id: string) => api.post(`/api/v1/node-apps/${id}/restart`),
  status: async (id: string): Promise<string | null> => {
    const body = unwrap<{ status?: string }>(await api.get(`/api/v1/node-apps/${id}/status`));
    return body?.status ?? null;
  },
  logs: async (id: string, lines = 200): Promise<string[]> => {
    const body = unwrap<{ logs?: string[] }>(
      await api.get(`/api/v1/node-apps/${id}/logs`, { params: { lines } })
    );
    return Array.isArray(body?.logs) ? (body!.logs as string[]) : [];
  },
  listEnv: async (id: string): Promise<NodeAppEnvironment[]> =>
    unwrapList<NodeAppEnvironment>(await api.get(`/api/v1/node-apps/${id}/environments`)),
  addEnv: (id: string, data: { key: string; value: string; is_secret?: boolean }) =>
    api.post(`/api/v1/node-apps/${id}/environments`, data),
  removeEnv: (id: string, envId: string) =>
    api.delete(`/api/v1/node-apps/${id}/environments/${envId}`),
};

/* ----------------------------------------------------------------------
 * Proxy projects - /api/v1/reverse-proxy (permission: reverseproxy)
 * -------------------------------------------------------------------- */

export const reverseProxyApi = {
  list: async (): Promise<ReverseProxy[]> =>
    unwrapList<ReverseProxy>(await api.get('/api/v1/reverse-proxy', { params: { limit: 100 } })),
  create: (data: CreateReverseProxyRequest) => api.post('/api/v1/reverse-proxy', data),
  update: (id: string, data: Record<string, unknown>) =>
    api.put(`/api/v1/reverse-proxy/${id}`, data),
  remove: (id: string) => api.delete(`/api/v1/reverse-proxy/${id}`),
  accessLogs: async (id: string): Promise<ReverseProxyAccessLog[]> =>
    unwrapList<ReverseProxyAccessLog>(
      await api.get(`/api/v1/reverse-proxy/${id}/access-logs`, { params: { limit: 100 } })
    ),
};

/* ----------------------------------------------------------------------
 * Deployments - /api/v1/git-deployments (permission: terminal:execute)
 * -------------------------------------------------------------------- */

export const siteDeploymentApi = {
  list: async (): Promise<GitDeployment[]> =>
    unwrapList<GitDeployment>(await api.get('/api/v1/git-deployments', { params: { limit: 200 } })),
};

/* ----------------------------------------------------------------------
 * WordPress records - /api/v1/wordpress (permission: wordpress)
 * -------------------------------------------------------------------- */

export const wordpressApi = {
  list: async (): Promise<WordPressSite[]> =>
    unwrapList<WordPressSite>(await api.get('/api/v1/wordpress')),
};

/* ----------------------------------------------------------------------
 * PHP-FPM pools - /api/v1/php/pools (permission: php)
 * -------------------------------------------------------------------- */

export const phpPoolApi = {
  listForWebsite: async (websiteId: string): Promise<PHPPool[]> =>
    unwrapList<PHPPool>(await api.get('/api/v1/php/pools', { params: { website_id: websiteId } })),
  update: (id: string, data: UpdatePHPPoolRequest) => api.put(`/api/v1/php/pools/${id}`, data),
};

/* ----------------------------------------------------------------------
 * Files - /api/v1/files (permission: website)
 * -------------------------------------------------------------------- */

export const siteFilesApi = {
  /**
   * The size of one document root. The backend runs `du -sh`, so this walks the
   * whole tree - it is asked for on demand, never for every row on load.
   */
  diskUsage: async (path: string): Promise<DiskUsage | null> =>
    unwrap<DiskUsage>(await api.get('/api/v1/files/disk-usage', { params: { path } })),
  /** Recursive delete, jailed to the file manager's base path by the backend. */
  remove: (path: string) => api.post('/api/v1/files/delete', { path }),
};

/**
 * Routes the Go code defines and router.go does NOT mount, as of this change.
 *
 * They are listed so the next person can see what the screen is missing rather
 * than rediscovering it, and so nobody wires a button to one of them by
 * accident. Calling any of these returns 404.
 */
export const UNMOUNTED_ROUTES = [
  'GET /api/v1/php/system',
  'POST /api/v1/php/install, POST /api/v1/php/uninstall',
  'GET/PUT /api/v1/php/pools/:id/settings',
  'GET/PUT /api/v1/php/sites/:website_id/version',
  'POST /api/v1/wordpress/:id/install',
  'GET/PUT /api/v1/wordpress/:id/runtime',
  'GET /api/v1/wordpress/:id/plugins/live, POST /api/v1/wordpress/:id/plugins/update',
  'GET /api/v1/wordpress/:id/themes/live, POST /api/v1/wordpress/:id/themes/update',
  'GET /api/v1/wordpress/:id/core/version, POST /api/v1/wordpress/:id/core/update',
  'POST /api/v1/wordpress/:id/search-replace, POST /api/v1/wordpress/:id/users/password',
  'GET/POST/DELETE /api/v1/wordpress/:id/staging, POST /api/v1/wordpress/:id/staging/push',
] as const;
