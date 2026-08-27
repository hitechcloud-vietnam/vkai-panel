'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  GitBranch,
  Plus,
  Trash2,
  Edit,
  Play,
  RotateCcw,
  X,
  Search,
  Loader2,
  AlertTriangle,
  RefreshCw,
  FileText,
  Clock,
  ExternalLink,
  CheckCircle2,
  XCircle,
  Timer,
  Loader,
} from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { deploymentApi, api } from '@/services/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface GitDeployment {
  id: string;
  tenant_id: string;
  server_id: string;
  website_id: string | null;
  name: string;
  repository_url: string;
  branch: string;
  deploy_path: string;
  deploy_key: string;
  webhook_secret: string;
  webhook_url: string;
  auto_deploy: boolean;
  deploy_script: string;
  pre_deploy_hook: string;
  post_deploy_hook: string;
  environment: Record<string, string>;
  status: string;
  is_active: boolean;
  last_deploy_at: string | null;
  last_commit_hash: string;
  created_at: string;
  updated_at: string;
}

interface DeploymentLog {
  id: string;
  deployment_id: string;
  tenant_id: string;
  commit_hash: string;
  commit_msg: string;
  author: string;
  status: string;
  output: string;
  error: string;
  duration: number;
  created_at: string;
}

interface InfrastructureServer {
  id: string;
  name: string;
  hostname: string;
  ip_address: string;
  status: string;
}

interface DeploymentFormData {
  server_id: string;
  name: string;
  repository_url: string;
  branch: string;
  deploy_path: string;
  deploy_key: string;
  deploy_script: string;
  pre_deploy_hook: string;
  post_deploy_hook: string;
  auto_deploy: boolean;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const EMPTY_FORM: DeploymentFormData = {
  server_id: '',
  name: '',
  repository_url: '',
  branch: 'main',
  deploy_path: '',
  deploy_key: '',
  deploy_script: '',
  pre_deploy_hook: '',
  post_deploy_hook: '',
  auto_deploy: false,
};

const INPUT_CLASS =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus-visible:ring-1 focus-visible:ring-brand-500 focus-visible:ring-offset-0 focus:outline-none';
const TEXTAREA_CLASS =
  'flex w-full resize-none rounded-md border border-gray-300 bg-white px-3 py-2 font-mono text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500';
const TH_CLASS =
  'text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500';
const LABEL_CLASS = 'block text-sm font-medium text-gray-700 mb-1.5';

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function DeploymentsPage() {
  // Data
  const [deployments, setDeployments] = useState<GitDeployment[]>([]);
  const [servers, setServers] = useState<InfrastructureServer[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Search
  const [search, setSearch] = useState('');

  // Modal state
  const [showForm, setShowForm] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showLogsModal, setShowLogsModal] = useState(false);
  const [editingDeployment, setEditingDeployment] = useState<GitDeployment | null>(null);

  // Form state
  const [form, setForm] = useState<DeploymentFormData>(EMPTY_FORM);
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  // Delete state
  const [deleteTarget, setDeleteTarget] = useState<GitDeployment | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Deploy state
  const [deployingId, setDeployingId] = useState<string | null>(null);

  // Logs state
  const [logsDeployment, setLogsDeployment] = useState<GitDeployment | null>(null);
  const [logs, setLogs] = useState<DeploymentLog[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsTotal, setLogsTotal] = useState(0);

  // Toast
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // -------------------------------------------------------------------------
  // Data fetching
  // -------------------------------------------------------------------------

  const fetchDeployments = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const [deployRes, serversRes] = await Promise.allSettled([
        deploymentApi.list({ limit: 100, offset: 0 }),
        api.get('/api/v1/servers'),
      ]);

      if (deployRes.status === 'fulfilled') {
        const list = deployRes.value?.data?.deployments;
        setDeployments(Array.isArray(list) ? list : []);
      } else {
        const err: any = deployRes.reason;
        console.error('Failed to load deployments:', err);
        setDeployments([]);
        setError(err?.response?.data?.error || 'Failed to load deployments');
      }

      if (serversRes.status === 'fulfilled') {
        const list = serversRes.value?.data?.data;
        setServers(Array.isArray(list) ? list : []);
      } else {
        console.error('Failed to load servers:', serversRes.reason);
        setServers([]);
      }
    } catch (err: any) {
      console.error('Failed to load deployments:', err);
      setDeployments([]);
      setServers([]);
      setError(err?.response?.data?.error || 'Failed to load deployments');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchDeployments();
  }, [fetchDeployments]);

  // Auto-dismiss toast
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 4000);
    return () => clearTimeout(t);
  }, [toast]);

  // -------------------------------------------------------------------------
  // Helpers
  // -------------------------------------------------------------------------

  const safeDeployments: GitDeployment[] = Array.isArray(deployments) ? deployments : [];
  const safeServers: InfrastructureServer[] = Array.isArray(servers) ? servers : [];
  const safeLogs: DeploymentLog[] = Array.isArray(logs) ? logs : [];

  const getServerName = (serverId: string) => {
    if (!serverId) return '—';
    const server = safeServers.find((s) => s?.id === serverId);
    return server?.name || serverId.slice(0, 8) + '…';
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'success':
      case 'deployed':
        return (
          <Badge className="gap-1 rounded-md border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 hover:bg-emerald-50">
            <CheckCircle2 size={12} />
            {status}
          </Badge>
        );
      case 'failed':
      case 'error':
        return (
          <Badge className="gap-1 rounded-md border border-red-200 bg-red-50 px-2 py-0.5 text-xs font-medium text-red-700 hover:bg-red-50">
            <XCircle size={12} />
            {status}
          </Badge>
        );
      case 'deploying':
      case 'building':
        return (
          <Badge className="gap-1 rounded-md border border-amber-200 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700 hover:bg-amber-50">
            <Loader size={12} className="animate-spin" />
            {status}
          </Badge>
        );
      case 'pending':
        return (
          <Badge className="gap-1 rounded-md border border-gray-200 bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 hover:bg-gray-100">
            <Timer size={12} />
            {status}
          </Badge>
        );
      default:
        return (
          <Badge className="rounded-md border border-gray-200 bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 hover:bg-gray-100">
            {status || 'unknown'}
          </Badge>
        );
    }
  };

  const formatDate = (dateStr: string | null) => {
    if (!dateStr) return '—';
    const d = new Date(dateStr);
    if (isNaN(d.getTime())) return '—';
    return (
      d.toLocaleDateString() +
      ' ' +
      d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    );
  };

  const formatDuration = (seconds: number) => {
    if (!seconds || seconds <= 0) return '—';
    if (seconds < 60) return `${seconds}s`;
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return `${m}m ${s}s`;
  };

  const extractRepoName = (url: string) => {
    if (!url) return '—';
    try {
      const parts = url.replace(/\.git$/, '').split('/');
      return parts[parts.length - 1] || url;
    } catch {
      return url;
    }
  };

  // -------------------------------------------------------------------------
  // Filtered data
  // -------------------------------------------------------------------------

  const filteredDeployments = safeDeployments.filter((d) => {
    if (!d) return false;
    if (!search) return true;
    const q = search.toLowerCase();
    return (
      (d.name || '').toLowerCase().includes(q) ||
      (d.repository_url || '').toLowerCase().includes(q) ||
      (d.branch || '').toLowerCase().includes(q) ||
      (d.status || '').toLowerCase().includes(q)
    );
  });

  // -------------------------------------------------------------------------
  // CRUD handlers
  // -------------------------------------------------------------------------

  const openCreateForm = () => {
    setEditingDeployment(null);
    setForm(EMPTY_FORM);
    setFormError(null);
    setShowForm(true);
  };

  const openEditForm = (deployment: GitDeployment) => {
    setEditingDeployment(deployment);
    setForm({
      server_id: deployment?.server_id || '',
      name: deployment?.name || '',
      repository_url: deployment?.repository_url || '',
      branch: deployment?.branch || '',
      deploy_path: deployment?.deploy_path || '',
      deploy_key: deployment?.deploy_key || '',
      deploy_script: deployment?.deploy_script || '',
      pre_deploy_hook: deployment?.pre_deploy_hook || '',
      post_deploy_hook: deployment?.post_deploy_hook || '',
      auto_deploy: Boolean(deployment?.auto_deploy),
    });
    setFormError(null);
    setShowForm(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);

    if (!form.name || !form.repository_url || !form.deploy_path || !form.server_id) {
      setFormError('Name, repository URL, deploy path, and server are required');
      return;
    }

    setSubmitting(true);
    try {
      if (editingDeployment) {
        await deploymentApi.update(editingDeployment.id, {
          name: form.name,
          repository_url: form.repository_url,
          branch: form.branch,
          deploy_path: form.deploy_path,
          deploy_key: form.deploy_key,
          deploy_script: form.deploy_script,
          pre_deploy_hook: form.pre_deploy_hook,
          post_deploy_hook: form.post_deploy_hook,
          auto_deploy: form.auto_deploy,
        });
        setToast({ type: 'success', message: 'Deployment updated successfully' });
      } else {
        await deploymentApi.create({
          server_id: form.server_id,
          name: form.name,
          repository_url: form.repository_url,
          branch: form.branch || 'main',
          deploy_path: form.deploy_path,
          deploy_key: form.deploy_key,
          deploy_script: form.deploy_script,
          pre_deploy_hook: form.pre_deploy_hook,
          post_deploy_hook: form.post_deploy_hook,
          auto_deploy: form.auto_deploy,
        });
        setToast({ type: 'success', message: 'Deployment created successfully' });
      }
      setShowForm(false);
      setEditingDeployment(null);
      setForm(EMPTY_FORM);
      fetchDeployments();
    } catch (err: any) {
      setFormError(err?.response?.data?.error || 'Failed to save deployment');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDeploy = async (deployment: GitDeployment) => {
    setDeployingId(deployment.id);
    try {
      await deploymentApi.deploy(deployment.id, {});
      setToast({ type: 'success', message: `Deployment triggered for ${deployment.name}` });
      fetchDeployments();
    } catch (err: any) {
      setToast({
        type: 'error',
        message: err?.response?.data?.error || 'Failed to trigger deployment',
      });
    } finally {
      setDeployingId(null);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await deploymentApi.delete(deleteTarget.id);
      setToast({ type: 'success', message: `Deployment "${deleteTarget.name}" deleted` });
      setShowDeleteConfirm(false);
      setDeleteTarget(null);
      fetchDeployments();
    } catch (err: any) {
      setToast({
        type: 'error',
        message: err?.response?.data?.error || 'Failed to delete deployment',
      });
    } finally {
      setDeleting(false);
    }
  };

  // -------------------------------------------------------------------------
  // Logs
  // -------------------------------------------------------------------------

  const openLogs = async (deployment: GitDeployment) => {
    setLogsDeployment(deployment);
    setShowLogsModal(true);
    setLogsLoading(true);
    setLogs([]);
    try {
      const res = await deploymentApi.listLogs(deployment.id, { limit: 50, offset: 0 });
      const list = res?.data?.logs;
      setLogs(Array.isArray(list) ? list : []);
      setLogsTotal(typeof res?.data?.total === 'number' ? res.data.total : 0);
    } catch (err: any) {
      console.error('Failed to load logs:', err);
      setLogs([]);
      setLogsTotal(0);
      setToast({
        type: 'error',
        message: err?.response?.data?.error || 'Failed to load deployment logs',
      });
    } finally {
      setLogsLoading(false);
    }
  };

  const handleClearLogs = async () => {
    if (!logsDeployment) return;
    try {
      await deploymentApi.clearLogs(logsDeployment.id);
      setLogs([]);
      setLogsTotal(0);
      setToast({ type: 'success', message: 'Deployment logs cleared' });
    } catch (err: any) {
      setToast({
        type: 'error',
        message: err?.response?.data?.error || 'Failed to clear logs',
      });
    }
  };

  // -------------------------------------------------------------------------
  // Loading state
  // -------------------------------------------------------------------------

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="animate-spin text-brand-600" size={32} aria-hidden="true" />
        <span className="sr-only">Loading deployments</span>
      </div>
    );
  }

  // -------------------------------------------------------------------------
  // Stats
  // -------------------------------------------------------------------------

  const successCount = safeDeployments.filter(
    (d) => d?.status === 'success' || d?.status === 'deployed'
  ).length;
  const failedCount = safeDeployments.filter(
    (d) => d?.status === 'failed' || d?.status === 'error'
  ).length;
  const activeCount = safeDeployments.filter(
    (d) => d?.status === 'deploying' || d?.status === 'building'
  ).length;

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Git Deployments</h1>
          <p className="mt-1 text-sm text-gray-600">
            Manage Git deployments and CI/CD pipelines
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Button
            onClick={fetchDeployments}
            variant="outline"
            size="sm"
            className="rounded-md border border-gray-300 bg-white text-sm font-medium text-gray-700 hover:bg-gray-50"
          >
            <RefreshCw size={16} className="mr-2" />
            Refresh
          </Button>
          <Button
            onClick={openCreateForm}
            className="rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700"
          >
            <Plus size={16} className="mr-2" />
            Add Deployment
          </Button>
        </div>
      </div>

      {/* Error banner */}
      {error && (
        <div className="flex items-center gap-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-red-700">
          <AlertTriangle size={18} />
          <span className="text-sm">{error}</span>
          <Button
            onClick={fetchDeployments}
            variant="outline"
            size="sm"
            className="ml-auto rounded-md border border-red-300 bg-white text-sm font-medium text-red-700 hover:bg-red-50"
          >
            <RefreshCw size={14} className="mr-2" />
            Retry
          </Button>
        </div>
      )}

      {/* Stats */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <Card className="border border-gray-200 bg-white shadow-sm">
          <CardContent className="p-5">
            <div className="flex items-center gap-3">
              <div className="rounded-md bg-brand-50 p-2">
                <GitBranch className="text-brand-600" size={20} aria-hidden="true" />
              </div>
              <div>
                <p className="text-sm text-gray-600">Total</p>
                <p className="text-2xl font-semibold text-gray-900">{safeDeployments.length}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="border border-gray-200 bg-white shadow-sm">
          <CardContent className="p-5">
            <div className="flex items-center gap-3">
              <div className="rounded-md bg-emerald-50 p-2">
                <CheckCircle2 className="text-emerald-600" size={20} aria-hidden="true" />
              </div>
              <div>
                <p className="text-sm text-gray-600">Successful</p>
                <p className="text-2xl font-semibold text-gray-900">{successCount}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="border border-gray-200 bg-white shadow-sm">
          <CardContent className="p-5">
            <div className="flex items-center gap-3">
              <div className="rounded-md bg-red-50 p-2">
                <XCircle className="text-red-600" size={20} aria-hidden="true" />
              </div>
              <div>
                <p className="text-sm text-gray-600">Failed</p>
                <p className="text-2xl font-semibold text-gray-900">{failedCount}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="border border-gray-200 bg-white shadow-sm">
          <CardContent className="p-5">
            <div className="flex items-center gap-3">
              <div className="rounded-md bg-amber-50 p-2">
                <Loader className="text-amber-600" size={20} aria-hidden="true" />
              </div>
              <div>
                <p className="text-sm text-gray-600">In Progress</p>
                <p className="text-2xl font-semibold text-gray-900">{activeCount}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Deployments Table */}
      <Card className="border border-gray-200 bg-white shadow-sm">
        <CardHeader className="flex flex-row items-center justify-between space-y-0 border-b border-gray-200 px-5 py-4">
          <CardTitle className="text-sm font-semibold text-gray-900">Deployments</CardTitle>
          <div className="relative">
            <Search
              className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
              size={16}
              aria-hidden="true"
            />
            <Input
              aria-label="Search deployments"
              placeholder="Search deployments..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className={`${INPUT_CLASS} w-64 pl-10`}
            />
          </div>
        </CardHeader>
        <CardContent className="p-0">
          {filteredDeployments.length === 0 ? (
            <div className="py-16 text-center">
              <GitBranch className="mx-auto text-gray-300" size={48} aria-hidden="true" />
              <h3 className="mt-4 text-sm font-semibold text-gray-900">No deployments</h3>
              <p className="mt-1 text-sm text-gray-600">
                {search
                  ? 'No deployments match your search'
                  : 'Set up your first deployment to get started'}
              </p>
              {!search && (
                <Button
                  onClick={openCreateForm}
                  className="mt-4 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700"
                >
                  <Plus size={16} className="mr-2" />
                  Add Deployment
                </Button>
              )}
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead className="bg-gray-50">
                  <tr className="border-b border-gray-200">
                    <th className={TH_CLASS}>Name</th>
                    <th className={TH_CLASS}>Repository</th>
                    <th className={TH_CLASS}>Branch</th>
                    <th className={TH_CLASS}>Server</th>
                    <th className={TH_CLASS}>Status</th>
                    <th className={TH_CLASS}>Last Deploy</th>
                    <th className={`${TH_CLASS} text-right`}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredDeployments.map((deployment) => (
                    <tr
                      key={deployment.id}
                      className="border-b border-gray-100 hover:bg-gray-50"
                    >
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <GitBranch className="text-gray-400" size={16} aria-hidden="true" />
                          <div>
                            <span className="block text-sm font-medium text-gray-900">
                              {deployment?.name || '—'}
                            </span>
                            {deployment?.auto_deploy && (
                              <span className="flex items-center gap-1 text-xs text-gray-500">
                                <RotateCcw size={10} />
                                Auto-deploy
                              </span>
                            )}
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1">
                          <span className="max-w-[200px] truncate font-mono text-sm text-gray-700">
                            {extractRepoName(deployment?.repository_url)}
                          </span>
                          {deployment?.repository_url && (
                            <a
                              href={deployment.repository_url}
                              target="_blank"
                              rel="noopener noreferrer"
                              aria-label={`Open repository for ${deployment?.name || 'deployment'}`}
                              className="rounded text-gray-400 hover:text-gray-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                            >
                              <ExternalLink size={12} />
                            </a>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <Badge className="rounded-md border border-gray-200 bg-gray-100 px-2 py-0.5 font-mono text-xs font-medium text-gray-600 hover:bg-gray-100">
                          <GitBranch size={10} className="mr-1" />
                          {deployment?.branch || '—'}
                        </Badge>
                      </td>
                      <td className="px-4 py-3 text-sm text-gray-700">
                        {getServerName(deployment?.server_id)}
                      </td>
                      <td className="px-4 py-3">{getStatusBadge(deployment?.status)}</td>
                      <td className="px-4 py-3 text-sm text-gray-700">
                        <div className="flex items-center gap-1">
                          <Clock size={12} className="text-gray-400" aria-hidden="true" />
                          {formatDate(deployment?.last_deploy_at ?? null)}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleDeploy(deployment)}
                            disabled={deployingId === deployment.id}
                            className="text-emerald-600 hover:bg-emerald-50 hover:text-emerald-700"
                            title="Deploy"
                            aria-label={`Deploy ${deployment?.name || 'deployment'}`}
                          >
                            {deployingId === deployment.id ? (
                              <Loader2 size={16} className="animate-spin" />
                            ) : (
                              <Play size={16} />
                            )}
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => openLogs(deployment)}
                            className="text-brand-600 hover:bg-brand-50 hover:text-brand-700"
                            title="View Logs"
                            aria-label={`View logs for ${deployment?.name || 'deployment'}`}
                          >
                            <FileText size={16} />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => openEditForm(deployment)}
                            className="text-gray-600 hover:bg-gray-100 hover:text-gray-900"
                            title="Edit"
                            aria-label={`Edit ${deployment?.name || 'deployment'}`}
                          >
                            <Edit size={16} />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => {
                              setDeleteTarget(deployment);
                              setShowDeleteConfirm(true);
                            }}
                            className="text-red-600 hover:bg-red-50 hover:text-red-700"
                            title="Delete"
                            aria-label={`Delete ${deployment?.name || 'deployment'}`}
                          >
                            <Trash2 size={16} />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* ================================================================== */}
      {/* Create / Edit Deployment Modal                                      */}
      {/* ================================================================== */}
      {showForm && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40"
          onClick={() => setShowForm(false)}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label={editingDeployment ? 'Edit deployment' : 'Add deployment'}
            className="max-h-[90vh] w-full max-w-2xl overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <h2 className="text-sm font-semibold text-gray-900">
                {editingDeployment ? 'Edit Deployment' : 'Add Deployment'}
              </h2>
              <button
                type="button"
                onClick={() => setShowForm(false)}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X size={18} />
              </button>
            </div>
            <form onSubmit={handleSubmit} className="space-y-4 px-5 py-4">
              {formError && (
                <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
                  {formError}
                </div>
              )}

              {/* Server & Name */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label htmlFor="deployment-server" className={LABEL_CLASS}>
                    Server *
                  </label>
                  <select
                    id="deployment-server"
                    value={form.server_id}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, server_id: e.target.value }))
                    }
                    className="h-10 w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
                  >
                    <option value="">Select a server</option>
                    {safeServers.map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.name} ({s.ip_address || s.hostname})
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label htmlFor="deployment-name" className={LABEL_CLASS}>
                    Name *
                  </label>
                  <Input
                    id="deployment-name"
                    value={form.name}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, name: e.target.value }))
                    }
                    placeholder="my-app"
                    className={INPUT_CLASS}
                  />
                </div>
              </div>

              {/* Repository URL */}
              <div>
                <label htmlFor="deployment-repo" className={LABEL_CLASS}>
                  Repository URL *
                </label>
                <Input
                  id="deployment-repo"
                  value={form.repository_url}
                  onChange={(e) =>
                    setForm((prev) => ({ ...prev, repository_url: e.target.value }))
                  }
                  placeholder="https://github.com/user/repo.git"
                  className={INPUT_CLASS}
                />
              </div>

              {/* Branch & Deploy Path */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label htmlFor="deployment-branch" className={LABEL_CLASS}>
                    Branch
                  </label>
                  <Input
                    id="deployment-branch"
                    value={form.branch}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, branch: e.target.value }))
                    }
                    placeholder="main"
                    className={INPUT_CLASS}
                  />
                </div>
                <div>
                  <label htmlFor="deployment-path" className={LABEL_CLASS}>
                    Deploy Path *
                  </label>
                  <Input
                    id="deployment-path"
                    value={form.deploy_path}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, deploy_path: e.target.value }))
                    }
                    placeholder="/var/www/my-app"
                    className={INPUT_CLASS}
                  />
                </div>
              </div>

              {/* Deploy Key */}
              <div>
                <label htmlFor="deployment-key" className={LABEL_CLASS}>
                  Deploy Key (SSH Private Key)
                </label>
                <textarea
                  id="deployment-key"
                  value={form.deploy_key}
                  onChange={(e) =>
                    setForm((prev) => ({ ...prev, deploy_key: e.target.value }))
                  }
                  placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                  rows={3}
                  className={TEXTAREA_CLASS}
                />
              </div>

              {/* Deploy Script */}
              <div>
                <label htmlFor="deployment-script" className={LABEL_CLASS}>
                  Deploy Script
                </label>
                <textarea
                  id="deployment-script"
                  value={form.deploy_script}
                  onChange={(e) =>
                    setForm((prev) => ({ ...prev, deploy_script: e.target.value }))
                  }
                  placeholder="npm install && npm run build"
                  rows={3}
                  className={TEXTAREA_CLASS}
                />
              </div>

              {/* Hooks */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label htmlFor="deployment-pre-hook" className={LABEL_CLASS}>
                    Pre-Deploy Hook
                  </label>
                  <Input
                    id="deployment-pre-hook"
                    value={form.pre_deploy_hook}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, pre_deploy_hook: e.target.value }))
                    }
                    placeholder="echo 'pre-deploy'"
                    className={`${INPUT_CLASS} font-mono text-xs`}
                  />
                </div>
                <div>
                  <label htmlFor="deployment-post-hook" className={LABEL_CLASS}>
                    Post-Deploy Hook
                  </label>
                  <Input
                    id="deployment-post-hook"
                    value={form.post_deploy_hook}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, post_deploy_hook: e.target.value }))
                    }
                    placeholder="systemctl restart my-app"
                    className={`${INPUT_CLASS} font-mono text-xs`}
                  />
                </div>
              </div>

              {/* Auto Deploy */}
              <div className="flex items-center gap-3">
                <input
                  type="checkbox"
                  id="auto_deploy"
                  checked={form.auto_deploy}
                  onChange={(e) =>
                    setForm((prev) => ({ ...prev, auto_deploy: e.target.checked }))
                  }
                  className="h-4 w-4 rounded border-gray-300 bg-white text-brand-600 focus:ring-1 focus:ring-brand-500"
                />
                <label htmlFor="auto_deploy" className="text-sm text-gray-700">
                  Enable auto-deploy on push
                </label>
              </div>

              {/* Actions */}
              <div className="flex justify-end gap-3 border-t border-gray-200 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowForm(false)}
                  className="rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={submitting}
                  className="rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-50"
                >
                  {submitting ? (
                    <Loader2 size={16} className="mr-2 animate-spin" />
                  ) : editingDeployment ? (
                    <Edit size={16} className="mr-2" />
                  ) : (
                    <Plus size={16} className="mr-2" />
                  )}
                  {editingDeployment ? 'Update Deployment' : 'Create Deployment'}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* Delete Confirmation Modal                                           */}
      {/* ================================================================== */}
      {showDeleteConfirm && deleteTarget && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40"
          onClick={() => setShowDeleteConfirm(false)}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Confirm deletion"
            className="w-full max-w-md rounded-lg border border-gray-200 bg-white shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center gap-3 border-b border-gray-200 px-5 py-4">
              <div className="rounded-md bg-red-50 p-2">
                <AlertTriangle className="text-red-600" size={20} aria-hidden="true" />
              </div>
              <h2 className="text-sm font-semibold text-gray-900">Confirm Deletion</h2>
            </div>
            <div className="px-5 py-4">
              <p className="text-sm text-gray-600">
                Are you sure you want to delete deployment{' '}
                <span className="font-semibold text-gray-900">
                  &quot;{deleteTarget?.name || ''}&quot;
                </span>
                ?
                <span className="mt-1 block text-sm text-gray-500">
                  This will remove the deployment configuration. Deployed files on the server will
                  not be affected.
                </span>
                <span className="mt-1 block text-sm text-gray-500">
                  This action cannot be undone.
                </span>
              </p>
              <div className="mt-6 flex justify-end gap-3">
                <Button
                  variant="outline"
                  onClick={() => setShowDeleteConfirm(false)}
                  className="rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
                >
                  Cancel
                </Button>
                <Button
                  onClick={handleDelete}
                  disabled={deleting}
                  className="rounded-md bg-red-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
                >
                  {deleting ? (
                    <Loader2 size={16} className="mr-2 animate-spin" />
                  ) : (
                    <Trash2 size={16} className="mr-2" />
                  )}
                  Delete
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* Deployment Logs Modal                                               */}
      {/* ================================================================== */}
      {showLogsModal && logsDeployment && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40"
          onClick={() => setShowLogsModal(false)}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Deployment logs"
            className="flex max-h-[85vh] w-full max-w-4xl flex-col rounded-lg border border-gray-200 bg-white shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <div>
                <h2 className="text-sm font-semibold text-gray-900">Deployment Logs</h2>
                <p className="mt-1 text-sm text-gray-600">
                  {logsDeployment?.name || '—'} — {logsDeployment?.repository_url || '—'}
                </p>
              </div>
              <div className="flex items-center gap-2">
                {safeLogs.length > 0 && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleClearLogs}
                    className="rounded-md border border-red-300 bg-white text-sm font-medium text-red-700 hover:bg-red-50"
                  >
                    <Trash2 size={14} className="mr-1" />
                    Clear Logs
                  </Button>
                )}
                <button
                  type="button"
                  onClick={() => setShowLogsModal(false)}
                  aria-label="Close dialog"
                  className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                >
                  <X size={18} />
                </button>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto px-5 py-4">
              {logsLoading ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 className="animate-spin text-brand-600" size={32} aria-hidden="true" />
                  <span className="sr-only">Loading logs</span>
                </div>
              ) : safeLogs.length === 0 ? (
                <div className="py-12 text-center">
                  <FileText className="mx-auto text-gray-300" size={48} aria-hidden="true" />
                  <h3 className="mt-4 text-sm font-semibold text-gray-900">No deployment logs</h3>
                  <p className="mt-1 text-sm text-gray-600">
                    Trigger a deployment to see logs here
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {safeLogs.map((log) => (
                    <div
                      key={log.id}
                      className="overflow-hidden rounded-md border border-gray-200"
                    >
                      {/* Log header */}
                      <div className="flex items-center justify-between bg-gray-50 p-3">
                        <div className="flex items-center gap-3">
                          {getStatusBadge(log?.status)}
                          {log?.commit_hash && (
                            <span className="font-mono text-xs text-gray-500">
                              {log.commit_hash.slice(0, 8)}
                            </span>
                          )}
                          {log?.commit_msg && (
                            <span className="max-w-[300px] truncate text-sm text-gray-700">
                              {log.commit_msg}
                            </span>
                          )}
                        </div>
                        <div className="flex items-center gap-3 text-xs text-gray-500">
                          {log?.author && <span>{log.author}</span>}
                          {(log?.duration ?? 0) > 0 && (
                            <span className="flex items-center gap-1">
                              <Clock size={10} />
                              {formatDuration(log.duration)}
                            </span>
                          )}
                          <span>{formatDate(log?.created_at ?? null)}</span>
                        </div>
                      </div>

                      {/* Log output */}
                      {(log?.output || log?.error) && (
                        <div className="border-t border-gray-200 bg-white p-3">
                          <pre className="max-h-48 overflow-x-auto overflow-y-auto whitespace-pre-wrap font-mono text-xs text-gray-700">
                            {log.output}
                            {log.error && <span className="text-red-600">{log.error}</span>}
                          </pre>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Footer */}
            <div className="flex items-center justify-between border-t border-gray-200 px-5 py-4">
              <span className="text-sm text-gray-500">
                {logsTotal} total log{logsTotal !== 1 ? 's' : ''}
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => openLogs(logsDeployment)}
                className="rounded-md border border-gray-300 bg-white text-sm font-medium text-gray-700 hover:bg-gray-50"
              >
                <RefreshCw size={14} className="mr-1" />
                Refresh
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* ================================================================== */}
      {/* Toast                                                               */}
      {/* ================================================================== */}
      {toast && (
        <div className="fixed bottom-6 right-6 z-50" role="status">
          <div
            className={`rounded-md border px-4 py-3 text-sm shadow-lg ${
              toast.type === 'success'
                ? 'bg-emerald-50 border-emerald-200 text-emerald-700'
                : 'bg-red-50 border-red-200 text-red-700'
            }`}
          >
            {toast.message}
          </div>
        </div>
      )}
    </div>
  );
}
