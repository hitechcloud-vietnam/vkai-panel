'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  Container,
  Box,
  Network,
  HardDrive,
  Play,
  Square,
  RotateCcw,
  Trash2,
  RefreshCw,
  Search,
  Loader2,
  FileCode,
  AlertTriangle,
  X,
  Copy,
  CheckCircle,
} from 'lucide-react';
import { dockerApi } from '@/services/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface DockerContainer {
  id: string;
  name: string;
  image: string;
  status: string;
  state: string;
  ports: string;
  cpu: number;
  memory: number;
  memory_limit: number;
  created_at: string;
}

interface DockerImage {
  id: string;
  repository: string;
  tag: string;
  size: number;
  created_at: string;
}

interface DockerNetwork {
  id: string;
  name: string;
  driver: string;
  scope: string;
  created_at: string;
}

interface DockerVolume {
  id: string;
  name: string;
  driver: string;
  mountpoint: string;
  created_at: string;
}

interface DockerSummary {
  running_containers: number;
  stopped_containers: number;
  total_images: number;
  total_volumes: number;
  total_networks: number;
}

// ---------------------------------------------------------------------------
// Style tokens
// ---------------------------------------------------------------------------

const CARD = 'bg-white border border-gray-200 rounded-lg shadow-sm';
const CARD_HEADER = 'px-5 py-4 border-b border-gray-200 flex items-center justify-between gap-4';
const TH = 'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
const TD = 'px-4 py-3 text-sm text-gray-700';
const ROW = 'border-b border-gray-100 hover:bg-gray-50';
const BADGE = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium';
const INPUT =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none';
const LABEL = 'mb-1.5 block text-sm font-medium text-gray-700';
const BTN_PRIMARY =
  'inline-flex items-center gap-2 rounded-md bg-blue-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-1 disabled:opacity-50';
const BTN_SECONDARY =
  'inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-1';
const BTN_DANGER =
  'inline-flex items-center gap-2 rounded-md bg-red-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-red-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500 focus-visible:ring-offset-1 disabled:opacity-50';
const ICON_BTN =
  'inline-flex h-8 w-8 items-center justify-center rounded-md text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 disabled:opacity-50';
const ICON_DANGER =
  'inline-flex h-8 w-8 items-center justify-center rounded-md text-gray-500 hover:bg-red-50 hover:text-red-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500 disabled:opacity-50';

const TABS = [
  { id: 'containers', label: 'Containers', icon: Container },
  { id: 'images', label: 'Images', icon: Box },
  { id: 'networks', label: 'Networks', icon: Network },
  { id: 'volumes', label: 'Volumes', icon: HardDrive },
  { id: 'compose', label: 'Compose', icon: FileCode },
];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatBytes(bytes: number): string {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function formatDate(dateStr: string): string {
  if (!dateStr) return '-';
  try {
    const d = new Date(dateStr);
    if (Number.isNaN(d.getTime())) return dateStr;
    return d.toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  } catch {
    return dateStr;
  }
}

function statusBadgeClass(status: string): string {
  const s = (status || '').toLowerCase();
  if (s.includes('running') || s.includes('up')) return 'bg-emerald-50 text-emerald-700';
  if (s.includes('exited') || s.includes('dead') || s.includes('stopped')) return 'bg-red-50 text-red-700';
  if (s.includes('paused') || s.includes('created')) return 'bg-amber-50 text-amber-700';
  return 'bg-gray-100 text-gray-700';
}

function matches(value: string | undefined, term: string): boolean {
  return (value || '').toLowerCase().includes(term);
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function DockerPage() {
  // Data
  const [containers, setContainers] = useState<DockerContainer[]>([]);
  const [images, setImages] = useState<DockerImage[]>([]);
  const [networks, setNetworks] = useState<DockerNetwork[]>([]);
  const [volumes, setVolumes] = useState<DockerVolume[]>([]);
  const [summary, setSummary] = useState<DockerSummary>({
    running_containers: 0,
    stopped_containers: 0,
    total_images: 0,
    total_volumes: 0,
    total_networks: 0,
  });

  // UI state
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState('containers');
  const [search, setSearch] = useState('');
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  // Compose state
  const [composeName, setComposeName] = useState('');
  const [composeContent, setComposeContent] = useState('');
  const [composeDeploying, setComposeDeploying] = useState(false);

  // Toast
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // Delete confirmation
  const [deleteTarget, setDeleteTarget] = useState<{
    type: 'container' | 'image' | 'network' | 'volume';
    id: string;
    name: string;
  } | null>(null);
  const [deleting, setDeleting] = useState(false);

  // -------------------------------------------------------------------------
  // Data fetching
  // -------------------------------------------------------------------------

  const showToast = useCallback((type: 'success' | 'error', message: string) => {
    setToast({ type, message });
    setTimeout(() => setToast(null), 4000);
  }, []);

  const fetchAll = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const [summaryRes, containersRes, imagesRes, networksRes, volumesRes] = await Promise.all([
        dockerApi.getSummary(),
        dockerApi.listContainers(),
        dockerApi.listImages(),
        dockerApi.listNetworks(),
        dockerApi.listVolumes(),
      ]);
      setSummary({
        running_containers: summaryRes?.data?.data?.running_containers ?? 0,
        stopped_containers: summaryRes?.data?.data?.stopped_containers ?? 0,
        total_images: summaryRes?.data?.data?.total_images ?? 0,
        total_volumes: summaryRes?.data?.data?.total_volumes ?? 0,
        total_networks: summaryRes?.data?.data?.total_networks ?? 0,
      });
      setContainers(Array.isArray(containersRes?.data?.data) ? containersRes.data.data : []);
      setImages(Array.isArray(imagesRes?.data?.data) ? imagesRes.data.data : []);
      setNetworks(Array.isArray(networksRes?.data?.data) ? networksRes.data.data : []);
      setVolumes(Array.isArray(volumesRes?.data?.data) ? volumesRes.data.data : []);
    } catch (err: any) {
      console.error('Failed to load Docker data:', err);
      setError(err?.response?.data?.message || 'Failed to load Docker data');
    } finally {
      setLoading(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  // -------------------------------------------------------------------------
  // Container actions
  // -------------------------------------------------------------------------

  const handleContainerAction = async (action: 'start' | 'stop' | 'restart', id: string, name: string) => {
    try {
      setActionLoading(`${action}-${id}`);
      if (action === 'start') await dockerApi.startContainer(id);
      else if (action === 'stop') await dockerApi.stopContainer(id);
      else await dockerApi.restartContainer(id);
      showToast('success', `Container "${name}" ${action === 'start' ? 'started' : action === 'stop' ? 'stopped' : 'restarted'} successfully`);
      await fetchAll();
    } catch (err: any) {
      showToast('error', err?.response?.data?.message || `Failed to ${action} container`);
    } finally {
      setActionLoading(null);
    }
  };

  // -------------------------------------------------------------------------
  // Delete handler
  // -------------------------------------------------------------------------

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      setDeleting(true);
      const { type, id } = deleteTarget;
      if (type === 'container') await dockerApi.deleteContainer(id);
      else if (type === 'image') await dockerApi.deleteImage(id);
      else if (type === 'network') await dockerApi.deleteNetwork(id);
      else if (type === 'volume') await dockerApi.deleteVolume(id);
      showToast('success', `${type.charAt(0).toUpperCase() + type.slice(1)} "${deleteTarget.name}" deleted successfully`);
      setDeleteTarget(null);
      await fetchAll();
    } catch (err: any) {
      showToast('error', err?.response?.data?.message || `Failed to delete ${deleteTarget.type}`);
    } finally {
      setDeleting(false);
    }
  };

  // -------------------------------------------------------------------------
  // Compose actions
  // -------------------------------------------------------------------------

  const handleDeployCompose = async () => {
    if (!composeName.trim() || !composeContent.trim()) {
      showToast('error', 'Please provide both a stack name and YAML content');
      return;
    }
    try {
      setComposeDeploying(true);
      await dockerApi.deployCompose({ name: composeName, content: composeContent });
      showToast('success', `Compose stack "${composeName}" deployed successfully`);
      setComposeName('');
      setComposeContent('');
      await fetchAll();
    } catch (err: any) {
      showToast('error', err?.response?.data?.message || 'Failed to deploy compose stack');
    } finally {
      setComposeDeploying(false);
    }
  };

  const handleStopCompose = async (name: string) => {
    try {
      setActionLoading(`compose-stop-${name}`);
      await dockerApi.stopCompose({ name });
      showToast('success', `Compose stack "${name}" stopped`);
      await fetchAll();
    } catch (err: any) {
      showToast('error', err?.response?.data?.message || 'Failed to stop compose stack');
    } finally {
      setActionLoading(null);
    }
  };

  const handleCopyMountpoint = async (mountpoint: string) => {
    try {
      await navigator.clipboard.writeText(mountpoint);
      showToast('success', 'Mountpoint copied to clipboard');
    } catch {
      showToast('error', 'Unable to copy the mountpoint');
    }
  };

  // -------------------------------------------------------------------------
  // Filter helpers
  // -------------------------------------------------------------------------

  const term = search.toLowerCase();

  const filteredContainers = containers.filter(
    (c) => matches(c?.name, term) || matches(c?.image, term) || matches(c?.status, term)
  );

  const filteredImages = images.filter(
    (img) => matches(img?.repository, term) || matches(img?.tag, term)
  );

  const filteredNetworks = networks.filter(
    (n) => matches(n?.name, term) || matches(n?.driver, term)
  );

  const filteredVolumes = volumes.filter(
    (v) => matches(v?.name, term) || matches(v?.driver, term)
  );

  const composeContainers = containers.filter((c) => (c?.name || '').includes('_'));
  const stackNames = Array.from(new Set(composeContainers.map((c) => (c.name || '').split('_')[0])));

  // -------------------------------------------------------------------------
  // Loading state
  // -------------------------------------------------------------------------

  if (loading) {
    return (
      <div className={`${CARD} px-5 py-12 text-center text-sm text-gray-500`}>Loading…</div>
    );
  }

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  return (
    <div className="space-y-6">
      {/* Toast */}
      {toast && (
        <div
          className={`fixed right-4 top-4 z-50 flex items-center gap-2 rounded-md border px-4 py-3 text-sm font-medium shadow-lg ${
            toast.type === 'success'
              ? 'border-emerald-200 bg-emerald-50 text-emerald-700'
              : 'border-red-200 bg-red-50 text-red-700'
          }`}
        >
          {toast.type === 'success' ? <CheckCircle size={16} /> : <AlertTriangle size={16} />}
          {toast.message}
          <button type="button" aria-label="Dismiss notification" onClick={() => setToast(null)} className="ml-2 rounded-md p-0.5 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500">
            <X size={14} />
          </button>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {deleteTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40 p-4">
          <div className="w-full max-w-md rounded-lg border border-gray-200 bg-white shadow-lg">
            <div className="flex items-center gap-3 border-b border-gray-200 px-5 py-4">
              <div className="rounded-md bg-red-50 p-2">
                <AlertTriangle className="text-red-600" size={18} />
              </div>
              <h2 className="text-sm font-semibold text-gray-900">Confirm Deletion</h2>
            </div>
            <p className="px-5 py-5 text-sm text-gray-600">
              Are you sure you want to delete {deleteTarget.type}{' '}
              <span className="font-medium text-gray-900">&quot;{deleteTarget.name}&quot;</span>?
              This action cannot be undone.
            </p>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              <button type="button" onClick={() => setDeleteTarget(null)} className={BTN_SECONDARY}>
                Cancel
              </button>
              <button type="button" onClick={handleDelete} disabled={deleting} className={BTN_DANGER}>
                {deleting ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Trash2 className="h-4 w-4" />
                )}
                Delete
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Docker</h1>
          <p className="mt-1 text-sm text-gray-600">
            Manage containers, images, networks, volumes, and compose stacks
          </p>
        </div>
        <button type="button" onClick={fetchAll} className={BTN_SECONDARY}>
          <RefreshCw className="h-4 w-4" />
          Refresh
        </button>
      </div>

      {/* Error Banner */}
      {error && (
        <div className="flex items-center gap-3 rounded-md border border-red-200 bg-red-50 px-4 py-3">
          <AlertTriangle className="shrink-0 text-red-600" size={18} />
          <p className="text-sm text-red-700">{error}</p>
          <button
            type="button"
            aria-label="Dismiss error"
            onClick={() => setError(null)}
            className="ml-auto rounded-md p-1 text-red-600 hover:bg-red-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
          >
            <X size={16} />
          </button>
        </div>
      )}

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className={`${CARD} flex items-center gap-4 p-4`}>
          <div className="rounded-md bg-emerald-50 p-2.5">
            <Container className="text-emerald-600" size={22} />
          </div>
          <div>
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Running Containers</p>
            <p className="mt-1 text-2xl font-semibold text-gray-900">{summary.running_containers ?? 0}</p>
          </div>
        </div>
        <div className={`${CARD} flex items-center gap-4 p-4`}>
          <div className="rounded-md bg-red-50 p-2.5">
            <Square className="text-red-600" size={22} />
          </div>
          <div>
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Stopped Containers</p>
            <p className="mt-1 text-2xl font-semibold text-gray-900">{summary.stopped_containers ?? 0}</p>
          </div>
        </div>
        <div className={`${CARD} flex items-center gap-4 p-4`}>
          <div className="rounded-md bg-blue-50 p-2.5">
            <Box className="text-blue-600" size={22} />
          </div>
          <div>
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Total Images</p>
            <p className="mt-1 text-2xl font-semibold text-gray-900">{summary.total_images ?? 0}</p>
          </div>
        </div>
        <div className={`${CARD} flex items-center gap-4 p-4`}>
          <div className="rounded-md bg-sky-50 p-2.5">
            <HardDrive className="text-sky-600" size={22} />
          </div>
          <div>
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Total Volumes</p>
            <p className="mt-1 text-2xl font-semibold text-gray-900">{summary.total_volumes ?? 0}</p>
          </div>
        </div>
      </div>

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6 overflow-x-auto" aria-label="Docker sections">
          {TABS.map((tab) => (
            <button
              key={tab.id}
              type="button"
              aria-current={activeTab === tab.id ? 'page' : undefined}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 whitespace-nowrap border-b-2 px-1 pb-3 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                activeTab === tab.id
                  ? 'border-blue-600 text-blue-700'
                  : 'border-transparent text-gray-600 hover:border-gray-300 hover:text-gray-900'
              }`}
            >
              <tab.icon className="h-4 w-4" />
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {/* ================================================================ */}
      {/* CONTAINERS TAB                                                   */}
      {/* ================================================================ */}
      {activeTab === 'containers' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Containers</h2>
            <div className="relative w-64">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
              <input
                type="text"
                aria-label="Search containers"
                placeholder="Search containers..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className={`${INPUT} pl-9`}
              />
            </div>
          </div>
          {filteredContainers.length === 0 ? (
            <div className="px-5 py-12 text-center">
              <Container className="mx-auto text-gray-400" size={40} />
              <h3 className="mt-4 text-sm font-semibold text-gray-900">No containers found</h3>
              <p className="mt-1 text-sm text-gray-600">
                {search ? 'Try adjusting your search' : 'No containers are currently running'}
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead className="bg-gray-50 border-b border-gray-200">
                  <tr>
                    <th className={TH}>Name</th>
                    <th className={TH}>Image</th>
                    <th className={TH}>Status</th>
                    <th className={TH}>Ports</th>
                    <th className={`${TH} text-right`}>CPU</th>
                    <th className={`${TH} text-right`}>Memory</th>
                    <th className={`${TH} text-right`}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredContainers.map((container) => (
                    <tr key={container.id} className={ROW}>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <Container size={16} className="text-gray-400" />
                          <span className="text-sm font-medium text-gray-900">{container.name}</span>
                        </div>
                      </td>
                      <td className={`${TD} font-mono text-xs`}>{container.image || '-'}</td>
                      <td className="px-4 py-3">
                        <span className={`${BADGE} ${statusBadgeClass(container.status)}`}>
                          {container.status || 'unknown'}
                        </span>
                      </td>
                      <td className={`${TD} font-mono text-xs`}>{container.ports || '-'}</td>
                      <td className={`${TD} text-right`}>
                        {container.cpu != null ? `${container.cpu.toFixed(1)}%` : '-'}
                      </td>
                      <td className={`${TD} text-right`}>
                        {container.memory != null ? formatBytes(container.memory) : '-'}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1">
                          {container.state === 'running' ? (
                            <>
                              <button
                                type="button"
                                title="Stop"
                                aria-label={`Stop container ${container.name}`}
                                disabled={actionLoading === `stop-${container.id}`}
                                onClick={() => handleContainerAction('stop', container.id, container.name)}
                                className={ICON_BTN}
                              >
                                {actionLoading === `stop-${container.id}` ? (
                                  <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                  <Square className="h-4 w-4" />
                                )}
                              </button>
                              <button
                                type="button"
                                title="Restart"
                                aria-label={`Restart container ${container.name}`}
                                disabled={actionLoading === `restart-${container.id}`}
                                onClick={() => handleContainerAction('restart', container.id, container.name)}
                                className={ICON_BTN}
                              >
                                {actionLoading === `restart-${container.id}` ? (
                                  <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                  <RotateCcw className="h-4 w-4" />
                                )}
                              </button>
                            </>
                          ) : (
                            <button
                              type="button"
                              title="Start"
                              aria-label={`Start container ${container.name}`}
                              disabled={actionLoading === `start-${container.id}`}
                              onClick={() => handleContainerAction('start', container.id, container.name)}
                              className={ICON_BTN}
                            >
                              {actionLoading === `start-${container.id}` ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <Play className="h-4 w-4" />
                              )}
                            </button>
                          )}
                          <button
                            type="button"
                            title="Delete"
                            aria-label={`Delete container ${container.name}`}
                            onClick={() =>
                              setDeleteTarget({ type: 'container', id: container.id, name: container.name })
                            }
                            className={ICON_DANGER}
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* ================================================================ */}
      {/* IMAGES TAB                                                       */}
      {/* ================================================================ */}
      {activeTab === 'images' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Images</h2>
            <div className="relative w-64">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
              <input
                type="text"
                aria-label="Search images"
                placeholder="Search images..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className={`${INPUT} pl-9`}
              />
            </div>
          </div>
          {filteredImages.length === 0 ? (
            <div className="px-5 py-12 text-center">
              <Box className="mx-auto text-gray-400" size={40} />
              <h3 className="mt-4 text-sm font-semibold text-gray-900">No images found</h3>
              <p className="mt-1 text-sm text-gray-600">
                {search ? 'Try adjusting your search' : 'Pull an image to get started'}
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead className="bg-gray-50 border-b border-gray-200">
                  <tr>
                    <th className={TH}>Repository</th>
                    <th className={TH}>Tag</th>
                    <th className={TH}>Image ID</th>
                    <th className={`${TH} text-right`}>Size</th>
                    <th className={TH}>Created</th>
                    <th className={`${TH} text-right`}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredImages.map((image) => (
                    <tr key={image.id} className={ROW}>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <Box size={16} className="text-gray-400" />
                          <span className="text-sm font-medium text-gray-900">{image.repository}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`${BADGE} bg-gray-100 text-gray-700`}>{image.tag || '-'}</span>
                      </td>
                      <td className={`${TD} font-mono text-xs`}>
                        {image.id ? image.id.substring(0, 12) : '-'}
                      </td>
                      <td className={`${TD} text-right`}>{formatBytes(image.size)}</td>
                      <td className={TD} suppressHydrationWarning>{formatDate(image.created_at)}</td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1">
                          <button
                            type="button"
                            title="Delete Image"
                            aria-label={`Delete image ${image.repository}:${image.tag}`}
                            onClick={() =>
                              setDeleteTarget({
                                type: 'image',
                                id: image.id,
                                name: `${image.repository}:${image.tag}`,
                              })
                            }
                            className={ICON_DANGER}
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* ================================================================ */}
      {/* NETWORKS TAB                                                     */}
      {/* ================================================================ */}
      {activeTab === 'networks' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Networks</h2>
            <div className="relative w-64">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
              <input
                type="text"
                aria-label="Search networks"
                placeholder="Search networks..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className={`${INPUT} pl-9`}
              />
            </div>
          </div>
          {filteredNetworks.length === 0 ? (
            <div className="px-5 py-12 text-center">
              <Network className="mx-auto text-gray-400" size={40} />
              <h3 className="mt-4 text-sm font-semibold text-gray-900">No networks found</h3>
              <p className="mt-1 text-sm text-gray-600">
                {search ? 'Try adjusting your search' : 'No custom networks created yet'}
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead className="bg-gray-50 border-b border-gray-200">
                  <tr>
                    <th className={TH}>Name</th>
                    <th className={TH}>Driver</th>
                    <th className={TH}>Scope</th>
                    <th className={TH}>Network ID</th>
                    <th className={`${TH} text-right`}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredNetworks.map((network) => (
                    <tr key={network.id} className={ROW}>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <Network size={16} className="text-gray-400" />
                          <span className="text-sm font-medium text-gray-900">{network.name}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`${BADGE} bg-gray-100 text-gray-700`}>{network.driver || '-'}</span>
                      </td>
                      <td className={TD}>{network.scope || '-'}</td>
                      <td className={`${TD} font-mono text-xs`}>
                        {network.id ? network.id.substring(0, 12) : '-'}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1">
                          <button
                            type="button"
                            title="Delete Network"
                            aria-label={`Delete network ${network.name}`}
                            onClick={() =>
                              setDeleteTarget({ type: 'network', id: network.id, name: network.name })
                            }
                            className={ICON_DANGER}
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* ================================================================ */}
      {/* VOLUMES TAB                                                      */}
      {/* ================================================================ */}
      {activeTab === 'volumes' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Volumes</h2>
            <div className="relative w-64">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
              <input
                type="text"
                aria-label="Search volumes"
                placeholder="Search volumes..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className={`${INPUT} pl-9`}
              />
            </div>
          </div>
          {filteredVolumes.length === 0 ? (
            <div className="px-5 py-12 text-center">
              <HardDrive className="mx-auto text-gray-400" size={40} />
              <h3 className="mt-4 text-sm font-semibold text-gray-900">No volumes found</h3>
              <p className="mt-1 text-sm text-gray-600">
                {search ? 'Try adjusting your search' : 'No volumes created yet'}
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead className="bg-gray-50 border-b border-gray-200">
                  <tr>
                    <th className={TH}>Name</th>
                    <th className={TH}>Driver</th>
                    <th className={TH}>Mountpoint</th>
                    <th className={TH}>Created</th>
                    <th className={`${TH} text-right`}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredVolumes.map((volume) => (
                    <tr key={volume.id} className={ROW}>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <HardDrive size={16} className="text-gray-400" />
                          <span className="text-sm font-medium text-gray-900">{volume.name}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`${BADGE} bg-gray-100 text-gray-700`}>{volume.driver || '-'}</span>
                      </td>
                      <td className={`${TD} max-w-xs truncate font-mono text-xs`} title={volume.mountpoint}>
                        {volume.mountpoint || '-'}
                      </td>
                      <td className={TD} suppressHydrationWarning>{formatDate(volume.created_at)}</td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1">
                          {volume.mountpoint && (
                            <button
                              type="button"
                              title="Copy mountpoint"
                              aria-label={`Copy mountpoint for ${volume.name}`}
                              onClick={() => handleCopyMountpoint(volume.mountpoint)}
                              className={ICON_BTN}
                            >
                              <Copy className="h-4 w-4" />
                            </button>
                          )}
                          <button
                            type="button"
                            title="Delete Volume"
                            aria-label={`Delete volume ${volume.name}`}
                            onClick={() =>
                              setDeleteTarget({ type: 'volume', id: volume.id, name: volume.name })
                            }
                            className={ICON_DANGER}
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* ================================================================ */}
      {/* COMPOSE TAB                                                      */}
      {/* ================================================================ */}
      {activeTab === 'compose' && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Compose Editor */}
          <div className={CARD}>
            <div className="border-b border-gray-200 px-5 py-4">
              <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                <FileCode size={16} className="text-gray-500" />
                Docker Compose Editor
              </h2>
            </div>
            <div className="space-y-4 p-5">
              <div>
                <label htmlFor="compose-name" className={LABEL}>Stack Name</label>
                <input
                  id="compose-name"
                  type="text"
                  placeholder="my-stack"
                  value={composeName}
                  onChange={(e) => setComposeName(e.target.value)}
                  className={INPUT}
                />
              </div>
              <div>
                <label htmlFor="compose-yaml" className={LABEL}>Compose YAML</label>
                <textarea
                  id="compose-yaml"
                  value={composeContent}
                  onChange={(e) => setComposeContent(e.target.value)}
                  placeholder={`version: "3.8"\nservices:\n  web:\n    image: nginx:latest\n    ports:\n      - "8080:80"\n  db:\n    image: postgres:15\n    environment:\n      POSTGRES_PASSWORD: example`}
                  className={`${INPUT} h-72 resize-y font-mono`}
                  spellCheck={false}
                />
              </div>
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={handleDeployCompose}
                  disabled={composeDeploying || !composeName.trim() || !composeContent.trim()}
                  className={BTN_PRIMARY}
                >
                  {composeDeploying ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Play className="h-4 w-4" />
                  )}
                  Deploy
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setComposeName('');
                    setComposeContent('');
                  }}
                  className={BTN_SECONDARY}
                >
                  Clear
                </button>
              </div>
            </div>
          </div>

          {/* Running Stacks */}
          <div className={CARD}>
            <div className="border-b border-gray-200 px-5 py-4">
              <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                <Container size={16} className="text-gray-500" />
                Running Stacks
              </h2>
            </div>
            <div className="p-5">
              {composeContainers.length === 0 ? (
                <div className="py-12 text-center">
                  <FileCode className="mx-auto text-gray-400" size={40} />
                  <h3 className="mt-4 text-sm font-semibold text-gray-900">No compose stacks</h3>
                  <p className="mt-1 text-sm text-gray-600">
                    Deploy a compose stack using the editor
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {stackNames.map((stackName) => {
                    const stackContainers = containers.filter(
                      (c) => (c?.name || '').startsWith(stackName + '_')
                    );
                    const allRunning = stackContainers.every((c) => c.state === 'running');
                    return (
                      <div
                        key={stackName}
                        className="flex items-center justify-between rounded-md border border-gray-200 bg-gray-50 p-3"
                      >
                        <div className="flex items-center gap-3">
                          <FileCode size={18} className="text-gray-400" />
                          <div>
                            <p className="text-sm font-medium text-gray-900">{stackName}</p>
                            <p className="text-xs text-gray-500">
                              {stackContainers.length} container{stackContainers.length !== 1 ? 's' : ''}
                            </p>
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          <span className={`${BADGE} ${allRunning ? 'bg-emerald-50 text-emerald-700' : 'bg-amber-50 text-amber-700'}`}>
                            {allRunning ? 'Running' : 'Partial'}
                          </span>
                          <button
                            type="button"
                            title="Stop Stack"
                            aria-label={`Stop stack ${stackName}`}
                            disabled={actionLoading === `compose-stop-${stackName}`}
                            onClick={() => handleStopCompose(stackName)}
                            className={ICON_DANGER}
                          >
                            {actionLoading === `compose-stop-${stackName}` ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <Square className="h-4 w-4" />
                            )}
                          </button>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
