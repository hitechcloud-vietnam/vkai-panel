import axios, { AxiosResponse } from 'axios';

// Base URL an toan: mac dinh dung same-origin ('') de request di qua nginx/rewrites.
// Neu co NEXT_PUBLIC_API_URL thi bo dau '/' thua o cuoi de tranh '//api/v1'.
const RAW_API_URL = process.env.NEXT_PUBLIC_API_URL || '';
const API_URL = RAW_API_URL.replace(/\/+$/, '');

const REQUEST_TIMEOUT = 15000;

export const api = axios.create({
  baseURL: API_URL,
  timeout: REQUEST_TIMEOUT,
  headers: {
    'Content-Type': 'application/json',
  },
});

const isBrowser = () => typeof window !== 'undefined';

function readToken(key: string): string | null {
  if (!isBrowser()) return null;
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeToken(key: string, value: string) {
  if (!isBrowser()) return;
  try {
    window.localStorage.setItem(key, value);
  } catch {
    /* bo qua khi trinh duyet chan localStorage */
  }
}

function clearTokens() {
  if (!isBrowser()) return;
  try {
    window.localStorage.removeItem('access_token');
    window.localStorage.removeItem('refresh_token');
  } catch {
    /* bo qua khi trinh duyet chan localStorage */
  }
}

/**
 * Dieu huong ve /login mot cach an toan: chi chay tren trinh duyet va
 * khong lap vo han khi dang o san trang /login.
 */
function redirectToLogin() {
  if (!isBrowser()) return;
  const path = window.location.pathname || '';
  if (path === '/login' || path.startsWith('/login/')) return;
  window.location.href = '/login';
}

/**
 * Lay phan du lieu that su tu response mot cach an toan.
 * Ho tro ca dang { data: ... } lan body tra thang, va khong bao gio nem loi.
 */
export function unwrap<T = any>(res: any, fallback: T | null = null): T | null {
  const body = res?.data;
  if (body === null || body === undefined) return fallback;
  if (typeof body !== 'object') return fallback;
  if ('data' in body) {
    const inner = (body as Record<string, unknown>).data;
    return (inner === null || inner === undefined ? fallback : (inner as T));
  }
  return body as T;
}

/**
 * Lay danh sach an toan: Go serialize nil slice thanh null nen luon ep ve mang.
 */
export function unwrapList<T = any>(res: any): T[] {
  const value = unwrap<any>(res, null);
  return Array.isArray(value) ? (value as T[]) : [];
}

// Request interceptor to add auth token
api.interceptors.request.use(
  (config) => {
    const token = readToken('access_token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// Response interceptor to handle token refresh
api.interceptors.response.use(
  (response: AxiosResponse) => response,
  async (error) => {
    const originalRequest = error?.config;
    const status = error?.response?.status;
    const url: string = originalRequest?.url || '';
    const isAuthEndpoint =
      url.includes('/api/v1/auth/login') || url.includes('/api/v1/auth/refresh');

    if (status === 401 && originalRequest && !originalRequest._retry && !isAuthEndpoint) {
      originalRequest._retry = true;

      try {
        const refreshToken = readToken('refresh_token');
        if (!refreshToken) {
          throw new Error('No refresh token');
        }

        const response = await axios.post(
          `${API_URL}/api/v1/auth/refresh`,
          { refresh_token: refreshToken },
          { timeout: REQUEST_TIMEOUT, headers: { 'Content-Type': 'application/json' } }
        );

        const tokens = unwrap<any>(response, null);
        const accessToken = tokens?.access_token;
        const newRefreshToken = tokens?.refresh_token;

        if (!accessToken) {
          throw new Error('Refresh response missing access_token');
        }

        writeToken('access_token', accessToken);
        if (newRefreshToken) {
          writeToken('refresh_token', newRefreshToken);
        }

        originalRequest.headers = originalRequest.headers || {};
        originalRequest.headers.Authorization = `Bearer ${accessToken}`;
        return api(originalRequest);
      } catch {
        clearTokens();
        redirectToLogin();
      }
    }

    // Luon tra ve Promise bi reject de tung trang tu xu ly, khong nem loi ra ngoai
    // lam vo toan bo ung dung.
    return Promise.reject(error);
  }
);

// Auth API
export const authApi = {
  login: (data: { email: string; password: string }) =>
    api.post('/api/v1/auth/login', data),
  register: (data: { email: string; password: string; name: string }) =>
    api.post('/api/v1/auth/register', data),
  logout: () => api.post('/api/v1/auth/logout'),
  me: () => api.get('/api/v1/auth/me'),
  refresh: (data: { refresh_token: string }) =>
    api.post('/api/v1/auth/refresh', data),
};

// Server API
export const serverApi = {
  list: () => api.get('/api/v1/servers'),
  get: (id: string) => api.get(`/api/v1/servers/${id}`),
  create: (data: any) => api.post('/api/v1/servers', data),
  update: (id: string, data: any) => api.put(`/api/v1/servers/${id}`, data),
  delete: (id: string) => api.delete(`/api/v1/servers/${id}`),
  heartbeat: (id: string) => api.post(`/api/v1/servers/${id}/heartbeat`),
};

// Website API
export const websiteApi = {
  list: () => api.get('/api/v1/websites'),
  get: (id: string) => api.get(`/api/v1/websites/${id}`),
  create: (data: any) => api.post('/api/v1/websites', data),
  update: (id: string, data: any) => api.put(`/api/v1/websites/${id}`, data),
  delete: (id: string) => api.delete(`/api/v1/websites/${id}`),
};

// Database API
export const databaseApi = {
  list: () => api.get('/api/v1/databases'),
  get: (id: string) => api.get(`/api/v1/databases/${id}`),
  create: (data: any) => api.post('/api/v1/databases', data),
  delete: (id: string) => api.delete(`/api/v1/databases/${id}`),
};

// SSL API
export const sslApi = {
  list: () => api.get('/api/v1/ssl'),
  get: (id: string) => api.get(`/api/v1/ssl/${id}`),
  create: (data: any) => api.post('/api/v1/ssl', data),
  delete: (id: string) => api.delete(`/api/v1/ssl/${id}`),
};

// Cron API
export const cronApi = {
  list: () => api.get('/api/v1/cron'),
  get: (id: string) => api.get(`/api/v1/cron/${id}`),
  create: (data: any) => api.post('/api/v1/cron', data),
  update: (id: string, data: any) => api.put(`/api/v1/cron/${id}`, data),
  delete: (id: string) => api.delete(`/api/v1/cron/${id}`),
};

// Firewall API
export const firewallApi = {
  list: () => api.get('/api/v1/firewall'),
  get: (id: string) => api.get(`/api/v1/firewall/${id}`),
  create: (data: any) => api.post('/api/v1/firewall', data),
  update: (id: string, data: any) => api.put(`/api/v1/firewall/${id}`, data),
  delete: (id: string) => api.delete(`/api/v1/firewall/${id}`),
};

// Backup API
export const backupApi = {
  list: () => api.get('/api/v1/backups'),
  get: (id: string) => api.get(`/api/v1/backups/${id}`),
  create: (data: any) => api.post('/api/v1/backups', data),
  delete: (id: string) => api.delete(`/api/v1/backups/${id}`),
  restore: (id: string) => api.post(`/api/v1/backups/${id}/restore`),
};

// DNS API
export const dnsApi = {
  // Zones
  listZones: () => api.get('/api/v1/dns/zones'),
  getZone: (id: string) => api.get(`/api/v1/dns/zones/${id}`),
  createZone: (data: any) => api.post('/api/v1/dns/zones', data),
  updateZone: (id: string, data: any) => api.put(`/api/v1/dns/zones/${id}`, data),
  deleteZone: (id: string) => api.delete(`/api/v1/dns/zones/${id}`),
  // Records
  listRecords: (zoneId: string) => api.get(`/api/v1/dns/zones/${zoneId}/records`),
  getRecord: (id: string) => api.get(`/api/v1/dns/records/${id}`),
  createRecord: (zoneId: string, data: any) => api.post(`/api/v1/dns/zones/${zoneId}/records`, data),
  updateRecord: (id: string, data: any) => api.put(`/api/v1/dns/records/${id}`, data),
  deleteRecord: (id: string) => api.delete(`/api/v1/dns/records/${id}`),
};

// Monitoring API
export const monitoringApi = {
  getSystemInfo: () => api.get('/api/v1/monitoring/system'),
  getMetrics: (params?: any) => api.get('/api/v1/monitoring/metrics', { params }),
  recordMetric: (data: any) => api.post('/api/v1/monitoring/metrics', data),
  getServerMetrics: (serverId: string, params?: any) =>
    api.get(`/api/v1/monitoring/servers/${serverId}/metrics`, { params }),
  getLatestMetric: (serverId: string, metricType: string) =>
    api.get(`/api/v1/monitoring/servers/${serverId}/metrics/${metricType}/latest`),
  getAlerts: () => api.get('/api/v1/monitoring/alerts'),
  createAlert: (data: any) => api.post('/api/v1/monitoring/alerts', data),
  updateAlert: (id: string, data: any) => api.put(`/api/v1/monitoring/alerts/${id}`, data),
  deleteAlert: (id: string) => api.delete(`/api/v1/monitoring/alerts/${id}`),
  getAlertLogs: (params?: any) => api.get('/api/v1/monitoring/alert-logs', { params }),
  getDashboards: () => api.get('/api/v1/monitoring/dashboards'),
  createDashboard: (data: any) => api.post('/api/v1/monitoring/dashboards', data),
  updateDashboard: (id: string, data: any) => api.put(`/api/v1/monitoring/dashboards/${id}`, data),
  deleteDashboard: (id: string) => api.delete(`/api/v1/monitoring/dashboards/${id}`),
};

// Log API
export const logApi = {
  search: (params?: any) => api.get('/api/v1/logs/search', { params }),
  record: (data: any) => api.post('/api/v1/logs', data),
  cleanup: (data: any) => api.post('/api/v1/logs/cleanup', data),
  getSources: () => api.get('/api/v1/logs/sources'),
  createSource: (data: any) => api.post('/api/v1/logs/sources', data),
  updateSource: (id: string, data: any) => api.put(`/api/v1/logs/sources/${id}`, data),
  deleteSource: (id: string) => api.delete(`/api/v1/logs/sources/${id}`),
  getRotations: () => api.get('/api/v1/logs/rotations'),
  createRotation: (data: any) => api.post('/api/v1/logs/rotations', data),
  updateRotation: (id: string, data: any) => api.put(`/api/v1/logs/rotations/${id}`, data),
  deleteRotation: (id: string) => api.delete(`/api/v1/logs/rotations/${id}`),
};

// Notification API
export const notificationApi = {
  getNotifications: (params?: any) => api.get('/api/v1/notifications', { params }),
  markAsRead: (id: string) => api.put(`/api/v1/notifications/${id}/read`),
  deleteNotification: (id: string) => api.delete(`/api/v1/notifications/${id}`),
  getTemplates: () => api.get('/api/v1/notifications/templates'),
  createTemplate: (data: any) => api.post('/api/v1/notifications/templates', data),
  updateTemplate: (id: string, data: any) => api.put(`/api/v1/notifications/templates/${id}`, data),
  deleteTemplate: (id: string) => api.delete(`/api/v1/notifications/templates/${id}`),
  getChannels: () => api.get('/api/v1/notifications/channels'),
  createChannel: (data: any) => api.post('/api/v1/notifications/channels', data),
  updateChannel: (id: string, data: any) => api.put(`/api/v1/notifications/channels/${id}`, data),
  deleteChannel: (id: string) => api.delete(`/api/v1/notifications/channels/${id}`),
  getPreferences: () => api.get('/api/v1/notifications/preferences'),
  updatePreferences: (data: any) => api.put('/api/v1/notifications/preferences', data),
};

// Security API
export const securityApi = {
  // Scans
  listScans: (params?: any) => api.get('/api/v1/security/scans', { params }),
  getScan: (id: string) => api.get(`/api/v1/security/scans/${id}`),
  createScan: (data: any) => api.post('/api/v1/security/scans', data),
  deleteScan: (id: string) => api.delete(`/api/v1/security/scans/${id}`),
  // Vulnerabilities
  listVulnerabilities: (params?: any) => api.get('/api/v1/security/vulnerabilities', { params }),
  listVulnerabilitiesByScan: (scanId: string) => api.get(`/api/v1/security/scans/${scanId}/vulnerabilities`),
  getVulnerability: (id: string) => api.get(`/api/v1/security/vulnerabilities/${id}`),
  updateVulnerability: (id: string, data: any) => api.put(`/api/v1/security/vulnerabilities/${id}`, data),
  deleteVulnerability: (id: string) => api.delete(`/api/v1/security/vulnerabilities/${id}`),
  // Checks
  listChecksByScan: (scanId: string) => api.get(`/api/v1/security/scans/${scanId}/checks`),
  // Policies
  listPolicies: () => api.get('/api/v1/security/policies'),
  getPolicy: (id: string) => api.get(`/api/v1/security/policies/${id}`),
  createPolicy: (data: any) => api.post('/api/v1/security/policies', data),
  updatePolicy: (id: string, data: any) => api.put(`/api/v1/security/policies/${id}`, data),
  deletePolicy: (id: string) => api.delete(`/api/v1/security/policies/${id}`),
};

// Audit API
export const auditApi = {
  get: (id: string) => api.get(`/api/v1/audit/${id}`),
  search: (params?: any) => api.get('/api/v1/audit/search', { params }),
  getStats: () => api.get('/api/v1/audit/stats'),
  cleanup: (data: any) => api.post('/api/v1/audit/cleanup', data),
};

// Cluster API
export const clusterApi = {
  getClusters: () => api.get('/api/v1/clusters'),
  getCluster: (id: string) => api.get(`/api/v1/clusters/${id}`),
  createCluster: (data: any) => api.post('/api/v1/clusters', data),
  updateCluster: (id: string, data: any) => api.put(`/api/v1/clusters/${id}`, data),
  deleteCluster: (id: string) => api.delete(`/api/v1/clusters/${id}`),
  getNodes: (clusterId: string) => api.get(`/api/v1/clusters/${clusterId}/nodes`),
  addNode: (clusterId: string, data: any) => api.post(`/api/v1/clusters/${clusterId}/nodes`, data),
  updateNode: (clusterId: string, nodeId: string, data: any) =>
    api.put(`/api/v1/clusters/${clusterId}/nodes/${nodeId}`, data),
  removeNode: (clusterId: string, nodeId: string) =>
    api.delete(`/api/v1/clusters/${clusterId}/nodes/${nodeId}`),
  getLoadBalancers: (params?: any) => api.get('/api/v1/load-balancers', { params }),
  getLoadBalancer: (id: string) => api.get(`/api/v1/load-balancers/${id}`),
  createLoadBalancer: (data: any) => api.post('/api/v1/load-balancers', data),
  updateLoadBalancer: (id: string, data: any) => api.put(`/api/v1/load-balancers/${id}`, data),
  deleteLoadBalancer: (id: string) => api.delete(`/api/v1/load-balancers/${id}`),
  getHAPairs: () => api.get('/api/v1/ha-pairs'),
  getHAPair: (id: string) => api.get(`/api/v1/ha-pairs/${id}`),
  createHAPair: (data: any) => api.post('/api/v1/ha-pairs', data),
  updateHAPair: (id: string, data: any) => api.put(`/api/v1/ha-pairs/${id}`, data),
  deleteHAPair: (id: string) => api.delete(`/api/v1/ha-pairs/${id}`),
  triggerFailover: (id: string) => api.post(`/api/v1/ha-pairs/${id}/failover`),
};

// Job API
export const jobApi = {
  list: (params?: any) => api.get('/api/v1/jobs', { params }),
  get: (id: string) => api.get(`/api/v1/jobs/${id}`),
  getStats: () => api.get('/api/v1/jobs/stats'),
  getQueueStats: () => api.get('/api/v1/jobs/queue-stats'),
  delete: (id: string) => api.delete(`/api/v1/jobs/${id}`),
  cancel: (id: string) => api.post(`/api/v1/jobs/${id}/cancel`),
  retry: (id: string) => api.post(`/api/v1/jobs/${id}/retry`),
  enqueueBackup: (data: any) => api.post('/api/v1/jobs/backup', data),
  enqueueRestore: (data: any) => api.post('/api/v1/jobs/restore', data),
  enqueueDeploy: (data: any) => api.post('/api/v1/jobs/deploy', data),
  enqueueSSL: (data: any) => api.post('/api/v1/jobs/ssl', data),
  enqueueCleanup: (data: any) => api.post('/api/v1/jobs/cleanup', data),
  cleanupOld: (retentionDays?: number) =>
    api.post('/api/v1/jobs/cleanup-old', null, { params: { retention_days: retentionDays } }),
};

// Git Deployment API
export const deploymentApi = {
  list: (params?: { limit?: number; offset?: number }) =>
    api.get('/api/v1/git-deployments', { params }),
  get: (id: string) => api.get(`/api/v1/git-deployments/${id}`),
  create: (data: any) => api.post('/api/v1/git-deployments', data),
  update: (id: string, data: any) => api.put(`/api/v1/git-deployments/${id}`, data),
  delete: (id: string) => api.delete(`/api/v1/git-deployments/${id}`),
  deploy: (id: string, data?: { commit_hash?: string; force?: boolean }) =>
    api.post(`/api/v1/git-deployments/${id}/deploy`, data || {}),
  listLogs: (id: string, params?: { limit?: number; offset?: number }) =>
    api.get(`/api/v1/git-deployments/${id}/logs`, { params }),
  clearLogs: (id: string) => api.delete(`/api/v1/git-deployments/${id}/logs`),
  listByServer: (serverId: string) =>
    api.get(`/api/v1/git-deployments/server/${serverId}`),
};

// Config API
export const configApi = {
  listSnapshots: (params?: any) => api.get('/api/v1/config/snapshots', { params }),
  getSnapshot: (id: string) => api.get(`/api/v1/config/snapshots/${id}`),
  createSnapshot: (data: any) => api.post('/api/v1/config/snapshots', data),
  deleteSnapshot: (id: string) => api.delete(`/api/v1/config/snapshots/${id}`),
  rollback: (data: any) => api.post('/api/v1/config/rollback', data),
  getDiff: (id1: string, id2: string) => api.get('/api/v1/config/diff', { params: { id1, id2 } }),
  getHistory: (params: any) => api.get('/api/v1/config/history', { params }),
  getStats: () => api.get('/api/v1/config/stats'),
  cleanup: (keepVersions?: number) =>
    api.post('/api/v1/config/cleanup', null, { params: { keep_versions: keepVersions } }),
  validate: (data: any) => api.post('/api/v1/config/validate', data),
  listTemplates: (params?: any) => api.get('/api/v1/config/templates', { params }),
  getTemplate: (id: string) => api.get(`/api/v1/config/templates/${id}`),
  createTemplate: (data: any) => api.post('/api/v1/config/templates', data),
  updateTemplate: (id: string, data: any) => api.put(`/api/v1/config/templates/${id}`, data),
  deleteTemplate: (id: string) => api.delete(`/api/v1/config/templates/${id}`),
};

// Docker API
export const dockerApi = {
  // Summary
  getSummary: () => api.get('/api/v1/docker/summary'),
  // Containers
  listContainers: () => api.get('/api/v1/docker/containers'),
  getContainer: (id: string) => api.get(`/api/v1/docker/containers/${id}`),
  startContainer: (id: string) => api.post(`/api/v1/docker/containers/${id}/start`),
  stopContainer: (id: string) => api.post(`/api/v1/docker/containers/${id}/stop`),
  restartContainer: (id: string) => api.post(`/api/v1/docker/containers/${id}/restart`),
  deleteContainer: (id: string) => api.delete(`/api/v1/docker/containers/${id}`),
  // Images
  listImages: () => api.get('/api/v1/docker/images'),
  pullImage: (data: { repository: string; tag?: string }) =>
    api.post('/api/v1/docker/images/pull', data),
  deleteImage: (id: string) => api.delete(`/api/v1/docker/images/${id}`),
  // Networks
  listNetworks: () => api.get('/api/v1/docker/networks'),
  createNetwork: (data: { name: string; driver?: string }) =>
    api.post('/api/v1/docker/networks', data),
  deleteNetwork: (id: string) => api.delete(`/api/v1/docker/networks/${id}`),
  // Volumes
  listVolumes: () => api.get('/api/v1/docker/volumes'),
  createVolume: (data: { name: string; driver?: string }) =>
    api.post('/api/v1/docker/volumes', data),
  deleteVolume: (id: string) => api.delete(`/api/v1/docker/volumes/${id}`),
  // Compose
  listComposeStacks: () => api.get('/api/v1/docker/compose'),
  deployCompose: (data: { name: string; content: string }) =>
    api.post('/api/v1/docker/compose/deploy', data),
  stopCompose: (data: { name: string }) =>
    api.post('/api/v1/docker/compose/stop', data),
};

// API Keys API
export const apiKeyApi = {
  list: () => api.get('/api/v1/api-keys'),
  get: (id: string) => api.get(`/api/v1/api-keys/${id}`),
  create: (data: any) => api.post('/api/v1/api-keys', data),
  update: (id: string, data: any) => api.put(`/api/v1/api-keys/${id}`, data),
  delete: (id: string) => api.delete(`/api/v1/api-keys/${id}`),
};

// Panel Access Settings API (administrator only).
// These endpoints change the port, the security entrance, the IP allow list and
// the panel's own TLS certificate. A change that would move or restrict the
// panel answers 409 with a confirmation payload; repeat the call with
// `confirm: true` once the operator has been shown the new URL.
export const panelSettingsApi = {
  get: () => api.get('/api/v1/panel/settings'),
  update: (data: Record<string, unknown>) => api.put('/api/v1/panel/settings', data),
  regenerateEntrance: (confirm: boolean) =>
    api.post('/api/v1/panel/settings/entrance/regenerate', { confirm }),
  reissueCertificate: () => api.post('/api/v1/panel/settings/tls/reissue'),
};

// Panel version and self-upgrade API.
//
// Two things here are unlike the rest of this file.
//
// The version endpoint needs no token. It is what a client asks while the panel
// is restarting itself: the upgrade takes the API away and brings it back on a
// new release, and comparing the version before and after is the only reliable
// proof that the upgrade landed. It gets a short timeout of its own so a poll
// against a socket that is not accepting yet fails fast instead of stalling the
// whole watch loop.
//
// The progress endpoint is polled through the same restart, so it too uses a
// short timeout. A rejected promise from either of these is an expected state
// during an upgrade, not an error to show the operator - see UpgradeSection.
const UPGRADE_POLL_TIMEOUT = 6000;

export const upgradeApi = {
  /** Running version, build commit and build date. No authentication needed. */
  version: () => api.get('/api/v1/version', { timeout: UPGRADE_POLL_TIMEOUT }),
  /** Cached release status plus the current job. Administrator only. */
  status: () => api.get('/api/v1/upgrade/status'),
  /** Force a fresh check against the release source. Administrator only. */
  check: () => api.post('/api/v1/upgrade/check'),
  /** Begin an upgrade. Returns a job id immediately. Administrator only. */
  start: (version?: string) =>
    api.post('/api/v1/upgrade/start', version ? { version } : {}),
  /** Where one job has got to. Administrator only. */
  progress: (jobId: string) =>
    api.get(`/api/v1/upgrade/progress/${encodeURIComponent(jobId)}`, {
      timeout: UPGRADE_POLL_TIMEOUT,
    }),
};
