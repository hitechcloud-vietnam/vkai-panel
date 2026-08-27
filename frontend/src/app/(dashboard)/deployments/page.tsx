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
      const [deployRes, serversRes] = await Promise.all([
        deploymentApi.list({ limit: 100, offset: 0 }),
        api.get('/api/v1/servers'),
      ]);
      setDeployments(deployRes.data.deployments || []);
      setServers(serversRes.data.data || []);
    } catch (err: any) {
      console.error('Failed to load deployments:', err);
      setError(err.response?.data?.error || 'Failed to load deployments');
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

  const getServerName = (serverId: string) => {
    const server = servers.find((s) => s.id === serverId);
    return server?.name || serverId.slice(0, 8) + '…';
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'success':
      case 'deployed':
        return (
          <Badge variant="success" className="gap-1">
            <CheckCircle2 size={12} />
            {status}
          </Badge>
        );
      case 'failed':
      case 'error':
        return (
          <Badge variant="destructive" className="gap-1">
            <XCircle size={12} />
            {status}
          </Badge>
        );
      case 'deploying':
      case 'building':
        return (
          <Badge variant="warning" className="gap-1">
            <Loader size={12} className="animate-spin" />
            {status}
          </Badge>
        );
      case 'pending':
        return (
          <Badge variant="secondary" className="gap-1">
            <Timer size={12} />
            {status}
          </Badge>
        );
      default:
        return <Badge variant="secondary">{status || 'unknown'}</Badge>;
    }
  };

  const formatDate = (dateStr: string | null) => {
    if (!dateStr) return '—';
    const d = new Date(dateStr);
    return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  const formatDuration = (seconds: number) => {
    if (!seconds || seconds <= 0) return '—';
    if (seconds < 60) return `${seconds}s`;
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return `${m}m ${s}s`;
  };

  const extractRepoName = (url: string) => {
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

  const filteredDeployments = deployments.filter((d) => {
    if (!search) return true;
    const q = search.toLowerCase();
    return (
      d.name.toLowerCase().includes(q) ||
      d.repository_url.toLowerCase().includes(q) ||
      d.branch.toLowerCase().includes(q) ||
      d.status.toLowerCase().includes(q)
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
      server_id: deployment.server_id,
      name: deployment.name,
      repository_url: deployment.repository_url,
      branch: deployment.branch,
      deploy_path: deployment.deploy_path,
      deploy_key: deployment.deploy_key,
      deploy_script: deployment.deploy_script,
      pre_deploy_hook: deployment.pre_deploy_hook,
      post_deploy_hook: deployment.post_deploy_hook,
      auto_deploy: deployment.auto_deploy,
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
      setFormError(err.response?.data?.error || 'Failed to save deployment');
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
        message: err.response?.data?.error || 'Failed to trigger deployment',
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
        message: err.response?.data?.error || 'Failed to delete deployment',
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
      setLogs(res.data.logs || []);
      setLogsTotal(res.data.total || 0);
    } catch (err: any) {
      console.error('Failed to load logs:', err);
      setToast({
        type: 'error',
        message: err.response?.data?.error || 'Failed to load deployment logs',
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
        message: err.response?.data?.error || 'Failed to clear logs',
      });
    }
  };

  // -------------------------------------------------------------------------
  // Loading / Error states
  // -------------------------------------------------------------------------

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="animate-spin text-primary-500" size={32} />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-4">
        <AlertTriangle className="text-red-400" size={48} />
        <p className="text-dark-300 text-lg">{error}</p>
        <Button onClick={fetchDeployments} variant="outline">
          <RefreshCw size={16} className="mr-2" />
          Retry
        </Button>
      </div>
    );
  }

  // -------------------------------------------------------------------------
  // Stats
  // -------------------------------------------------------------------------

  const successCount = deployments.filter((d) => d.status === 'success' || d.status === 'deployed').length;
  const failedCount = deployments.filter((d) => d.status === 'failed' || d.status === 'error').length;
  const activeCount = deployments.filter((d) => d.status === 'deploying' || d.status === 'building').length;

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Git Deployments</h1>
          <p className="text-dark-400 mt-1">
            Manage Git deployments and CI/CD pipelines
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Button onClick={fetchDeployments} variant="outline" size="sm">
            <RefreshCw size={16} className="mr-2" />
            Refresh
          </Button>
          <Button onClick={openCreateForm}>
            <Plus size={16} className="mr-2" />
            Add Deployment
          </Button>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Card className="bg-dark-900 border-dark-700">
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-blue-500/10 rounded-lg">
                <GitBranch className="text-blue-400" size={20} />
              </div>
              <div>
                <p className="text-sm text-dark-400">Total</p>
                <p className="text-2xl font-bold text-dark-50">{deployments.length}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-dark-900 border-dark-700">
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-green-500/10 rounded-lg">
                <CheckCircle2 className="text-green-400" size={20} />
              </div>
              <div>
                <p className="text-sm text-dark-400">Successful</p>
                <p className="text-2xl font-bold text-dark-50">{successCount}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-dark-900 border-dark-700">
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-red-500/10 rounded-lg">
                <XCircle className="text-red-400" size={20} />
              </div>
              <div>
                <p className="text-sm text-dark-400">Failed</p>
                <p className="text-2xl font-bold text-dark-50">{failedCount}</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-dark-900 border-dark-700">
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-yellow-500/10 rounded-lg">
                <Loader className="text-yellow-400" size={20} />
              </div>
              <div>
                <p className="text-sm text-dark-400">In Progress</p>
                <p className="text-2xl font-bold text-dark-50">{activeCount}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Deployments Table */}
      <Card className="bg-dark-900 border-dark-700">
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle className="text-dark-50 text-lg">Deployments</CardTitle>
          <div className="relative">
            <Search
              className="absolute left-3 top-1/2 -translate-y-1/2 text-dark-400"
              size={16}
            />
            <Input
              placeholder="Search deployments..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-10 bg-dark-800 border-dark-600 text-dark-100 w-64"
            />
          </div>
        </CardHeader>
        <CardContent>
          {filteredDeployments.length === 0 ? (
            <div className="text-center py-12">
              <GitBranch className="mx-auto text-dark-600" size={48} />
              <h3 className="mt-4 text-lg font-medium text-dark-300">
                No deployments
              </h3>
              <p className="mt-2 text-dark-500">
                {search
                  ? 'No deployments match your search'
                  : 'Set up your first deployment to get started'}
              </p>
              {!search && (
                <Button onClick={openCreateForm} className="mt-4">
                  <Plus size={16} className="mr-2" />
                  Add Deployment
                </Button>
              )}
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-dark-700">
                    <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                      Name
                    </th>
                    <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                      Repository
                    </th>
                    <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                      Branch
                    </th>
                    <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                      Server
                    </th>
                    <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                      Status
                    </th>
                    <th className="text-left py-3 px-4 text-dark-400 font-medium text-sm">
                      Last Deploy
                    </th>
                    <th className="text-right py-3 px-4 text-dark-400 font-medium text-sm">
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {filteredDeployments.map((deployment) => (
                    <tr
                      key={deployment.id}
                      className="border-b border-dark-800 hover:bg-dark-800/50 transition-colors"
                    >
                      <td className="py-3 px-4">
                        <div className="flex items-center gap-2">
                          <GitBranch className="text-blue-400" size={16} />
                          <div>
                            <span className="text-dark-100 font-medium block">
                              {deployment.name}
                            </span>
                            {deployment.auto_deploy && (
                              <span className="text-xs text-dark-500 flex items-center gap-1">
                                <RotateCcw size={10} />
                                Auto-deploy
                              </span>
                            )}
                          </div>
                        </div>
                      </td>
                      <td className="py-3 px-4">
                        <div className="flex items-center gap-1">
                          <span className="text-dark-200 text-sm font-mono truncate max-w-[200px]">
                            {extractRepoName(deployment.repository_url)}
                          </span>
                          <a
                            href={deployment.repository_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-dark-500 hover:text-dark-300 transition-colors"
                          >
                            <ExternalLink size={12} />
                          </a>
                        </div>
                      </td>
                      <td className="py-3 px-4">
                        <Badge variant="secondary" className="font-mono">
                          <GitBranch size={10} className="mr-1" />
                          {deployment.branch}
                        </Badge>
                      </td>
                      <td className="py-3 px-4 text-dark-300 text-sm">
                        {getServerName(deployment.server_id)}
                      </td>
                      <td className="py-3 px-4">
                        {getStatusBadge(deployment.status)}
                      </td>
                      <td className="py-3 px-4 text-dark-400 text-sm">
                        <div className="flex items-center gap-1">
                          <Clock size={12} />
                          {formatDate(deployment.last_deploy_at)}
                        </div>
                      </td>
                      <td className="py-3 px-4 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleDeploy(deployment)}
                            disabled={deployingId === deployment.id}
                            className="text-green-400 hover:text-green-300 hover:bg-green-500/10"
                            title="Deploy"
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
                            className="text-blue-400 hover:text-blue-300 hover:bg-blue-500/10"
                            title="View Logs"
                          >
                            <FileText size={16} />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => openEditForm(deployment)}
                            className="text-yellow-400 hover:text-yellow-300 hover:bg-yellow-500/10"
                            title="Edit"
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
                            className="text-red-400 hover:text-red-300 hover:bg-red-500/10"
                            title="Delete"
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
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
          onClick={() => setShowForm(false)}
        >
          <div
            className="bg-dark-900 border border-dark-700 rounded-lg w-full max-w-2xl p-6 shadow-xl max-h-[90vh] overflow-y-auto"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-lg font-semibold text-dark-50">
                {editingDeployment ? 'Edit Deployment' : 'Add Deployment'}
              </h2>
              <button
                onClick={() => setShowForm(false)}
                className="text-dark-400 hover:text-dark-200 transition-colors"
              >
                <X size={20} />
              </button>
            </div>
            <form onSubmit={handleSubmit} className="space-y-4">
              {formError && (
                <div className="p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-red-400 text-sm">
                  {formError}
                </div>
              )}

              {/* Server & Name */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1">
                    Server *
                  </label>
                  <select
                    value={form.server_id}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, server_id: e.target.value }))
                    }
                    className="flex h-10 w-full rounded-md border border-dark-600 bg-dark-800 px-3 py-2 text-sm text-dark-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
                  >
                    <option value="">Select a server</option>
                    {servers.map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.name} ({s.ip_address || s.hostname})
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1">
                    Name *
                  </label>
                  <Input
                    value={form.name}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, name: e.target.value }))
                    }
                    placeholder="my-app"
                    className="bg-dark-800 border-dark-600 text-dark-100"
                  />
                </div>
              </div>

              {/* Repository URL */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  Repository URL *
                </label>
                <Input
                  value={form.repository_url}
                  onChange={(e) =>
                    setForm((prev) => ({ ...prev, repository_url: e.target.value }))
                  }
                  placeholder="https://github.com/user/repo.git"
                  className="bg-dark-800 border-dark-600 text-dark-100"
                />
              </div>

              {/* Branch & Deploy Path */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1">
                    Branch
                  </label>
                  <Input
                    value={form.branch}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, branch: e.target.value }))
                    }
                    placeholder="main"
                    className="bg-dark-800 border-dark-600 text-dark-100"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1">
                    Deploy Path *
                  </label>
                  <Input
                    value={form.deploy_path}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, deploy_path: e.target.value }))
                    }
                    placeholder="/var/www/my-app"
                    className="bg-dark-800 border-dark-600 text-dark-100"
                  />
                </div>
              </div>

              {/* Deploy Key */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  Deploy Key (SSH Private Key)
                </label>
                <textarea
                  value={form.deploy_key}
                  onChange={(e) =>
                    setForm((prev) => ({ ...prev, deploy_key: e.target.value }))
                  }
                  placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                  rows={3}
                  className="flex w-full rounded-md border border-dark-600 bg-dark-800 px-3 py-2 text-sm text-dark-100 font-mono placeholder:text-dark-500 focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
                />
              </div>

              {/* Deploy Script */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1">
                  Deploy Script
                </label>
                <textarea
                  value={form.deploy_script}
                  onChange={(e) =>
                    setForm((prev) => ({ ...prev, deploy_script: e.target.value }))
                  }
                  placeholder="npm install && npm run build"
                  rows={3}
                  className="flex w-full rounded-md border border-dark-600 bg-dark-800 px-3 py-2 text-sm text-dark-100 font-mono placeholder:text-dark-500 focus:outline-none focus:ring-2 focus:ring-blue-500 resize-none"
                />
              </div>

              {/* Hooks */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1">
                    Pre-Deploy Hook
                  </label>
                  <Input
                    value={form.pre_deploy_hook}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, pre_deploy_hook: e.target.value }))
                    }
                    placeholder="echo 'pre-deploy'"
                    className="bg-dark-800 border-dark-600 text-dark-100 font-mono text-xs"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1">
                    Post-Deploy Hook
                  </label>
                  <Input
                    value={form.post_deploy_hook}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, post_deploy_hook: e.target.value }))
                    }
                    placeholder="systemctl restart my-app"
                    className="bg-dark-800 border-dark-600 text-dark-100 font-mono text-xs"
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
                  className="h-4 w-4 rounded border-dark-600 bg-dark-800 text-blue-500 focus:ring-blue-500"
                />
                <label htmlFor="auto_deploy" className="text-sm text-dark-300">
                  Enable auto-deploy on push
                </label>
              </div>

              {/* Actions */}
              <div className="flex justify-end gap-3 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowForm(false)}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={submitting}>
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
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
          onClick={() => setShowDeleteConfirm(false)}
        >
          <div
            className="bg-dark-900 border border-dark-700 rounded-lg w-full max-w-md p-6 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-red-500/10 rounded-lg">
                <AlertTriangle className="text-red-400" size={24} />
              </div>
              <h2 className="text-lg font-semibold text-dark-50">
                Confirm Deletion
              </h2>
            </div>
            <p className="text-dark-300 mb-6">
              Are you sure you want to delete deployment{' '}
              <span className="text-dark-100 font-medium">
                &quot;{deleteTarget.name}&quot;
              </span>
              ?
              <span className="block text-dark-500 text-sm mt-1">
                This will remove the deployment configuration. Deployed files on the server will not be affected.
              </span>
              <span className="block text-dark-500 text-sm mt-1">
                This action cannot be undone.
              </span>
            </p>
            <div className="flex justify-end gap-3">
              <Button
                variant="outline"
                onClick={() => setShowDeleteConfirm(false)}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={handleDelete}
                disabled={deleting}
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
      )}

      {/* ================================================================== */}
      {/* Deployment Logs Modal                                               */}
      {/* ================================================================== */}
      {showLogsModal && logsDeployment && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
          onClick={() => setShowLogsModal(false)}
        >
          <div
            className="bg-dark-900 border border-dark-700 rounded-lg w-full max-w-4xl p-6 shadow-xl max-h-[85vh] flex flex-col"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-4">
              <div>
                <h2 className="text-lg font-semibold text-dark-50">
                  Deployment Logs
                </h2>
                <p className="text-dark-400 text-sm mt-1">
                  {logsDeployment.name} — {logsDeployment.repository_url}
                </p>
              </div>
              <div className="flex items-center gap-2">
                {logs.length > 0 && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleClearLogs}
                    className="text-red-400 hover:text-red-300 border-dark-600"
                  >
                    <Trash2 size={14} className="mr-1" />
                    Clear Logs
                  </Button>
                )}
                <button
                  onClick={() => setShowLogsModal(false)}
                  className="text-dark-400 hover:text-dark-200 transition-colors"
                >
                  <X size={20} />
                </button>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto">
              {logsLoading ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 className="animate-spin text-primary-500" size={32} />
                </div>
              ) : logs.length === 0 ? (
                <div className="text-center py-12">
                  <FileText className="mx-auto text-dark-600" size={48} />
                  <h3 className="mt-4 text-lg font-medium text-dark-300">
                    No deployment logs
                  </h3>
                  <p className="mt-2 text-dark-500">
                    Trigger a deployment to see logs here
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {logs.map((log) => (
                    <div
                      key={log.id}
                      className="border border-dark-700 rounded-lg overflow-hidden"
                    >
                      {/* Log header */}
                      <div className="flex items-center justify-between p-3 bg-dark-800/50">
                        <div className="flex items-center gap-3">
                          {getStatusBadge(log.status)}
                          {log.commit_hash && (
                            <span className="text-dark-400 text-xs font-mono">
                              {log.commit_hash.slice(0, 8)}
                            </span>
                          )}
                          {log.commit_msg && (
                            <span className="text-dark-300 text-sm truncate max-w-[300px]">
                              {log.commit_msg}
                            </span>
                          )}
                        </div>
                        <div className="flex items-center gap-3 text-dark-500 text-xs">
                          {log.author && <span>{log.author}</span>}
                          {log.duration > 0 && (
                            <span className="flex items-center gap-1">
                              <Clock size={10} />
                              {formatDuration(log.duration)}
                            </span>
                          )}
                          <span>{formatDate(log.created_at)}</span>
                        </div>
                      </div>

                      {/* Log output */}
                      {(log.output || log.error) && (
                        <div className="p-3 bg-dark-950">
                          <pre className="text-xs text-dark-300 font-mono whitespace-pre-wrap overflow-x-auto max-h-48 overflow-y-auto">
                            {log.output}
                            {log.error && (
                              <span className="text-red-400">{log.error}</span>
                            )}
                          </pre>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Footer */}
            <div className="flex items-center justify-between mt-4 pt-4 border-t border-dark-700">
              <span className="text-dark-500 text-sm">
                {logsTotal} total log{logsTotal !== 1 ? 's' : ''}
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => openLogs(logsDeployment)}
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
        <div className="fixed bottom-6 right-6 z-50 animate-in fade-in slide-in-from-bottom-2">
          <div
            className={`p-4 rounded-lg shadow-lg border ${
              toast.type === 'success'
                ? 'bg-green-900/90 border-green-700 text-green-200'
                : 'bg-red-900/90 border-red-700 text-red-200'
            }`}
          >
            {toast.message}
          </div>
        </div>
      )}
    </div>
  );
}
