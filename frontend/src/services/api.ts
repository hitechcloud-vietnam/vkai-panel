import axios from 'axios';

const API_URL = process.env.NEXT_PUBLIC_API_URL || '';

export const api = axios.create({
  baseURL: API_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor to add auth token
api.interceptors.request.use(
  (config) => {
    const token = typeof window !== 'undefined' ? localStorage.getItem('access_token') : null;
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
  (response) => response,
  async (error) => {
    const originalRequest = error.config;

    if (error.response?.status === 401 && !originalRequest._retry) {
      originalRequest._retry = true;

      try {
        const refreshToken = localStorage.getItem('refresh_token');
        if (!refreshToken) {
          throw new Error('No refresh token');
        }

        const response = await axios.post(`${API_URL}/api/v1/auth/refresh`, {
          refresh_token: refreshToken,
        });

        const { access_token, refresh_token } = response.data.data;
        localStorage.setItem('access_token', access_token);
        localStorage.setItem('refresh_token', refresh_token);

        originalRequest.headers.Authorization = `Bearer ${access_token}`;
        return api(originalRequest);
      } catch {
        localStorage.removeItem('access_token');
        localStorage.removeItem('refresh_token');
        window.location.href = '/login';
      }
    }

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
  list: () => api.get('/api/v1/dns'),
  get: (id: string) => api.get(`/api/v1/dns/${id}`),
  create: (data: any) => api.post('/api/v1/dns', data),
  update: (id: string, data: any) => api.put(`/api/v1/dns/${id}`, data),
  delete: (id: string) => api.delete(`/api/v1/dns/${id}`),
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
