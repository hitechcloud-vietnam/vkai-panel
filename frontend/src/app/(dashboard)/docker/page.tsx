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
} from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
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
// Helpers
// ---------------------------------------------------------------------------

function formatBytes(bytes: number): string {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function formatDate(dateStr: string): string {
  if (!dateStr) return '-';
  try {
    return new Date(dateStr).toLocaleDateString('en-US', {
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

function statusBadgeVariant(status: string): 'default' | 'secondary' | 'destructive' | 'success' | 'warning' | 'outline' {
  const s = status.toLowerCase();
  if (s.includes('running') || s.includes('up')) return 'success';
  if (s.includes('exited') || s.includes('dead') || s.includes('stopped')) return 'destructive';
  if (s.includes('paused') || s.includes('created')) return 'warning';
  return 'secondary';
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
      setSummary(summaryRes.data.data || summary);
      setContainers(containersRes.data.data || []);
      setImages(imagesRes.data.data || []);
      setNetworks(networksRes.data.data || []);
      setVolumes(volumesRes.data.data || []);
    } catch (err: any) {
      console.error('Failed to load Docker data:', err);
      setError(err.response?.data?.message || 'Failed to load Docker data');
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
      showToast('error', err.response?.data?.message || `Failed to ${action} container`);
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
      showToast('error', err.response?.data?.message || `Failed to delete ${deleteTarget.type}`);
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
      showToast('error', err.response?.data?.message || 'Failed to deploy compose stack');
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
      showToast('error', err.response?.data?.message || 'Failed to stop compose stack');
    } finally {
      setActionLoading(null);
    }
  };

  // -------------------------------------------------------------------------
  // Filter helpers
  // -------------------------------------------------------------------------

  const filteredContainers = containers.filter(
    (c) =>
      c.name.toLowerCase().includes(search.toLowerCase()) ||
      c.image.toLowerCase().includes(search.toLowerCase()) ||
      c.status.toLowerCase().includes(search.toLowerCase())
  );

  const filteredImages = images.filter(
    (img) =>
      img.repository.toLowerCase().includes(search.toLowerCase()) ||
      img.tag.toLowerCase().includes(search.toLowerCase())
  );

  const filteredNetworks = networks.filter(
    (n) =>
      n.name.toLowerCase().includes(search.toLowerCase()) ||
      n.driver.toLowerCase().includes(search.toLowerCase())
  );

  const filteredVolumes = volumes.filter(
    (v) =>
      v.name.toLowerCase().includes(search.toLowerCase()) ||
      v.driver.toLowerCase().includes(search.toLowerCase())
  );

  // -------------------------------------------------------------------------
  // Loading state
  // -------------------------------------------------------------------------

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
      </div>
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
          className={`fixed top-4 right-4 z-50 flex items-center gap-2 rounded-lg px-4 py-3 text-sm font-medium shadow-lg ${
            toast.type === 'success'
              ? 'bg-green-600 text-white'
              : 'bg-red-600 text-white'
          }`}
        >
          {toast.type === 'success' ? <Play size={16} /> : <AlertTriangle size={16} />}
          {toast.message}
          <button onClick={() => setToast(null)} className="ml-2">
            <X size={14} />
          </button>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {deleteTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
          <div className="bg-gray-900 border border-gray-700 rounded-xl p-6 w-full max-w-md shadow-2xl">
            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-red-500/10 rounded-lg">
                <AlertTriangle className="text-red-400" size={20} />
              </div>
              <h3 className="text-lg font-semibold text-gray-100">Confirm Deletion</h3>
            </div>
            <p className="text-gray-400 mb-6">
              Are you sure you want to delete {deleteTarget.type}{' '}
              <span className="text-gray-200 font-medium">&quot;{deleteTarget.name}&quot;</span>?
              This action cannot be undone.
            </p>
            <div className="flex justify-end gap-3">
              <Button
                variant="outline"
                onClick={() => setDeleteTarget(null)}
                className="border-gray-600 text-gray-300 hover:bg-gray-800"
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={handleDelete}
                disabled={deleting}
              >
                {deleting ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Trash2 className="mr-2 h-4 w-4" />
                )}
                Delete
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-100">Docker</h1>
          <p className="text-gray-400 mt-1">
            Manage containers, images, networks, volumes, and compose stacks
          </p>
        </div>
        <Button variant="outline" onClick={fetchAll} className="border-gray-700 text-gray-300 hover:bg-gray-800">
          <RefreshCw className="mr-2 h-4 w-4" />
          Refresh
        </Button>
      </div>

      {/* Error Banner */}
      {error && (
        <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-4 flex items-center gap-3">
          <AlertTriangle className="text-red-400 shrink-0" size={18} />
          <p className="text-red-300 text-sm">{error}</p>
          <button onClick={() => setError(null)} className="ml-auto text-red-400 hover:text-red-300">
            <X size={16} />
          </button>
        </div>
      )}

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card className="bg-gray-900 border-gray-800">
          <CardContent className="p-4 flex items-center gap-4">
            <div className="p-3 bg-green-500/10 rounded-lg">
              <Container className="text-green-400" size={24} />
            </div>
            <div>
              <p className="text-sm text-gray-400">Running Containers</p>
              <p className="text-2xl font-bold text-gray-100">{summary.running_containers}</p>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-gray-900 border-gray-800">
          <CardContent className="p-4 flex items-center gap-4">
            <div className="p-3 bg-red-500/10 rounded-lg">
              <Square className="text-red-400" size={24} />
            </div>
            <div>
              <p className="text-sm text-gray-400">Stopped Containers</p>
              <p className="text-2xl font-bold text-gray-100">{summary.stopped_containers}</p>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-gray-900 border-gray-800">
          <CardContent className="p-4 flex items-center gap-4">
            <div className="p-3 bg-blue-500/10 rounded-lg">
              <Box className="text-blue-400" size={24} />
            </div>
            <div>
              <p className="text-sm text-gray-400">Total Images</p>
              <p className="text-2xl font-bold text-gray-100">{summary.total_images}</p>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-gray-900 border-gray-800">
          <CardContent className="p-4 flex items-center gap-4">
            <div className="p-3 bg-purple-500/10 rounded-lg">
              <HardDrive className="text-purple-400" size={24} />
            </div>
            <div>
              <p className="text-sm text-gray-400">Total Volumes</p>
              <p className="text-2xl font-bold text-gray-100">{summary.total_volumes}</p>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Tabs */}
      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList className="bg-gray-800 border border-gray-700">
          <TabsTrigger value="containers" className="data-[state=active]:bg-gray-700 data-[state=active]:text-gray-100 text-gray-400">
            <Container className="mr-2 h-4 w-4" />
            Containers
          </TabsTrigger>
          <TabsTrigger value="images" className="data-[state=active]:bg-gray-700 data-[state=active]:text-gray-100 text-gray-400">
            <Box className="mr-2 h-4 w-4" />
            Images
          </TabsTrigger>
          <TabsTrigger value="networks" className="data-[state=active]:bg-gray-700 data-[state=active]:text-gray-100 text-gray-400">
            <Network className="mr-2 h-4 w-4" />
            Networks
          </TabsTrigger>
          <TabsTrigger value="volumes" className="data-[state=active]:bg-gray-700 data-[state=active]:text-gray-100 text-gray-400">
            <HardDrive className="mr-2 h-4 w-4" />
            Volumes
          </TabsTrigger>
          <TabsTrigger value="compose" className="data-[state=active]:bg-gray-700 data-[state=active]:text-gray-100 text-gray-400">
            <FileCode className="mr-2 h-4 w-4" />
            Compose
          </TabsTrigger>
        </TabsList>

        {/* ================================================================ */}
        {/* CONTAINERS TAB                                                   */}
        {/* ================================================================ */}
        <TabsContent value="containers">
          <Card className="bg-gray-900 border-gray-800">
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-gray-100 text-lg">Containers</CardTitle>
                <div className="relative w-64">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 h-4 w-4" />
                  <Input
                    placeholder="Search containers..."
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    className="pl-9 bg-gray-800 border-gray-700 text-gray-200 placeholder:text-gray-500"
                  />
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {filteredContainers.length === 0 ? (
                <div className="text-center py-12">
                  <Container className="mx-auto text-gray-600" size={48} />
                  <h3 className="mt-4 text-lg font-medium text-gray-400">No containers found</h3>
                  <p className="mt-2 text-gray-500 text-sm">
                    {search ? 'Try adjusting your search' : 'No containers are currently running'}
                  </p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-gray-800">
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Name</th>
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Image</th>
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Status</th>
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Ports</th>
                        <th className="text-right py-3 px-4 text-gray-400 font-medium">CPU</th>
                        <th className="text-right py-3 px-4 text-gray-400 font-medium">Memory</th>
                        <th className="text-right py-3 px-4 text-gray-400 font-medium">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredContainers.map((container) => (
                        <tr key={container.id} className="border-b border-gray-800/50 hover:bg-gray-800/30 transition-colors">
                          <td className="py-3 px-4">
                            <div className="flex items-center gap-2">
                              <Container size={16} className="text-gray-500" />
                              <span className="text-gray-200 font-medium">{container.name}</span>
                            </div>
                          </td>
                          <td className="py-3 px-4 text-gray-400 font-mono text-xs">{container.image}</td>
                          <td className="py-3 px-4">
                            <Badge variant={statusBadgeVariant(container.status)}>
                              {container.status}
                            </Badge>
                          </td>
                          <td className="py-3 px-4 text-gray-400 font-mono text-xs">{container.ports || '-'}</td>
                          <td className="py-3 px-4 text-right text-gray-300">
                            {container.cpu != null ? `${container.cpu.toFixed(1)}%` : '-'}
                          </td>
                          <td className="py-3 px-4 text-right text-gray-300">
                            {container.memory != null ? formatBytes(container.memory) : '-'}
                          </td>
                          <td className="py-3 px-4">
                            <div className="flex items-center justify-end gap-1">
                              {container.state === 'running' ? (
                                <>
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    title="Stop"
                                    disabled={actionLoading === `stop-${container.id}`}
                                    onClick={() => handleContainerAction('stop', container.id, container.name)}
                                    className="h-8 w-8 text-gray-400 hover:text-red-400 hover:bg-red-500/10"
                                  >
                                    {actionLoading === `stop-${container.id}` ? (
                                      <Loader2 className="h-4 w-4 animate-spin" />
                                    ) : (
                                      <Square className="h-4 w-4" />
                                    )}
                                  </Button>
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    title="Restart"
                                    disabled={actionLoading === `restart-${container.id}`}
                                    onClick={() => handleContainerAction('restart', container.id, container.name)}
                                    className="h-8 w-8 text-gray-400 hover:text-yellow-400 hover:bg-yellow-500/10"
                                  >
                                    {actionLoading === `restart-${container.id}` ? (
                                      <Loader2 className="h-4 w-4 animate-spin" />
                                    ) : (
                                      <RotateCcw className="h-4 w-4" />
                                    )}
                                  </Button>
                                </>
                              ) : (
                                <Button
                                  variant="ghost"
                                  size="icon"
                                  title="Start"
                                  disabled={actionLoading === `start-${container.id}`}
                                  onClick={() => handleContainerAction('start', container.id, container.name)}
                                  className="h-8 w-8 text-gray-400 hover:text-green-400 hover:bg-green-500/10"
                                >
                                  {actionLoading === `start-${container.id}` ? (
                                    <Loader2 className="h-4 w-4 animate-spin" />
                                  ) : (
                                    <Play className="h-4 w-4" />
                                  )}
                                </Button>
                              )}
                              <Button
                                variant="ghost"
                                size="icon"
                                title="Delete"
                                onClick={() =>
                                  setDeleteTarget({ type: 'container', id: container.id, name: container.name })
                                }
                                className="h-8 w-8 text-gray-400 hover:text-red-400 hover:bg-red-500/10"
                              >
                                <Trash2 className="h-4 w-4" />
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
        </TabsContent>

        {/* ================================================================ */}
        {/* IMAGES TAB                                                       */}
        {/* ================================================================ */}
        <TabsContent value="images">
          <Card className="bg-gray-900 border-gray-800">
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-gray-100 text-lg">Images</CardTitle>
                <div className="relative w-64">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 h-4 w-4" />
                  <Input
                    placeholder="Search images..."
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    className="pl-9 bg-gray-800 border-gray-700 text-gray-200 placeholder:text-gray-500"
                  />
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {filteredImages.length === 0 ? (
                <div className="text-center py-12">
                  <Box className="mx-auto text-gray-600" size={48} />
                  <h3 className="mt-4 text-lg font-medium text-gray-400">No images found</h3>
                  <p className="mt-2 text-gray-500 text-sm">
                    {search ? 'Try adjusting your search' : 'Pull an image to get started'}
                  </p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-gray-800">
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Repository</th>
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Tag</th>
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Image ID</th>
                        <th className="text-right py-3 px-4 text-gray-400 font-medium">Size</th>
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Created</th>
                        <th className="text-right py-3 px-4 text-gray-400 font-medium">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredImages.map((image) => (
                        <tr key={image.id} className="border-b border-gray-800/50 hover:bg-gray-800/30 transition-colors">
                          <td className="py-3 px-4">
                            <div className="flex items-center gap-2">
                              <Box size={16} className="text-gray-500" />
                              <span className="text-gray-200 font-medium">{image.repository}</span>
                            </div>
                          </td>
                          <td className="py-3 px-4">
                            <Badge variant="secondary" className="bg-gray-700 text-gray-300">
                              {image.tag}
                            </Badge>
                          </td>
                          <td className="py-3 px-4 text-gray-500 font-mono text-xs">
                            {image.id ? image.id.substring(0, 12) : '-'}
                          </td>
                          <td className="py-3 px-4 text-right text-gray-300">{formatBytes(image.size)}</td>
                          <td className="py-3 px-4 text-gray-400">{formatDate(image.created_at)}</td>
                          <td className="py-3 px-4">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="icon"
                                title="Delete Image"
                                onClick={() =>
                                  setDeleteTarget({
                                    type: 'image',
                                    id: image.id,
                                    name: `${image.repository}:${image.tag}`,
                                  })
                                }
                                className="h-8 w-8 text-gray-400 hover:text-red-400 hover:bg-red-500/10"
                              >
                                <Trash2 className="h-4 w-4" />
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
        </TabsContent>

        {/* ================================================================ */}
        {/* NETWORKS TAB                                                     */}
        {/* ================================================================ */}
        <TabsContent value="networks">
          <Card className="bg-gray-900 border-gray-800">
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-gray-100 text-lg">Networks</CardTitle>
                <div className="relative w-64">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 h-4 w-4" />
                  <Input
                    placeholder="Search networks..."
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    className="pl-9 bg-gray-800 border-gray-700 text-gray-200 placeholder:text-gray-500"
                  />
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {filteredNetworks.length === 0 ? (
                <div className="text-center py-12">
                  <Network className="mx-auto text-gray-600" size={48} />
                  <h3 className="mt-4 text-lg font-medium text-gray-400">No networks found</h3>
                  <p className="mt-2 text-gray-500 text-sm">
                    {search ? 'Try adjusting your search' : 'No custom networks created yet'}
                  </p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-gray-800">
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Name</th>
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Driver</th>
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Scope</th>
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Network ID</th>
                        <th className="text-right py-3 px-4 text-gray-400 font-medium">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredNetworks.map((network) => (
                        <tr key={network.id} className="border-b border-gray-800/50 hover:bg-gray-800/30 transition-colors">
                          <td className="py-3 px-4">
                            <div className="flex items-center gap-2">
                              <Network size={16} className="text-gray-500" />
                              <span className="text-gray-200 font-medium">{network.name}</span>
                            </div>
                          </td>
                          <td className="py-3 px-4">
                            <Badge variant="secondary" className="bg-gray-700 text-gray-300">
                              {network.driver}
                            </Badge>
                          </td>
                          <td className="py-3 px-4 text-gray-400">{network.scope}</td>
                          <td className="py-3 px-4 text-gray-500 font-mono text-xs">
                            {network.id ? network.id.substring(0, 12) : '-'}
                          </td>
                          <td className="py-3 px-4">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="icon"
                                title="Delete Network"
                                onClick={() =>
                                  setDeleteTarget({ type: 'network', id: network.id, name: network.name })
                                }
                                className="h-8 w-8 text-gray-400 hover:text-red-400 hover:bg-red-500/10"
                              >
                                <Trash2 className="h-4 w-4" />
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
        </TabsContent>

        {/* ================================================================ */}
        {/* VOLUMES TAB                                                      */}
        {/* ================================================================ */}
        <TabsContent value="volumes">
          <Card className="bg-gray-900 border-gray-800">
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-gray-100 text-lg">Volumes</CardTitle>
                <div className="relative w-64">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 h-4 w-4" />
                  <Input
                    placeholder="Search volumes..."
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    className="pl-9 bg-gray-800 border-gray-700 text-gray-200 placeholder:text-gray-500"
                  />
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {filteredVolumes.length === 0 ? (
                <div className="text-center py-12">
                  <HardDrive className="mx-auto text-gray-600" size={48} />
                  <h3 className="mt-4 text-lg font-medium text-gray-400">No volumes found</h3>
                  <p className="mt-2 text-gray-500 text-sm">
                    {search ? 'Try adjusting your search' : 'No volumes created yet'}
                  </p>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-gray-800">
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Name</th>
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Driver</th>
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Mountpoint</th>
                        <th className="text-left py-3 px-4 text-gray-400 font-medium">Created</th>
                        <th className="text-right py-3 px-4 text-gray-400 font-medium">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredVolumes.map((volume) => (
                        <tr key={volume.id} className="border-b border-gray-800/50 hover:bg-gray-800/30 transition-colors">
                          <td className="py-3 px-4">
                            <div className="flex items-center gap-2">
                              <HardDrive size={16} className="text-gray-500" />
                              <span className="text-gray-200 font-medium">{volume.name}</span>
                            </div>
                          </td>
                          <td className="py-3 px-4">
                            <Badge variant="secondary" className="bg-gray-700 text-gray-300">
                              {volume.driver}
                            </Badge>
                          </td>
                          <td className="py-3 px-4 text-gray-400 font-mono text-xs max-w-xs truncate" title={volume.mountpoint}>
                            {volume.mountpoint || '-'}
                          </td>
                          <td className="py-3 px-4 text-gray-400">{formatDate(volume.created_at)}</td>
                          <td className="py-3 px-4">
                            <div className="flex items-center justify-end gap-1">
                              {volume.mountpoint && (
                                <Button
                                  variant="ghost"
                                  size="icon"
                                  title="Copy mountpoint"
                                  onClick={() => {
                                    navigator.clipboard.writeText(volume.mountpoint);
                                    showToast('success', 'Mountpoint copied to clipboard');
                                  }}
                                  className="h-8 w-8 text-gray-400 hover:text-blue-400 hover:bg-blue-500/10"
                                >
                                  <Copy className="h-4 w-4" />
                                </Button>
                              )}
                              <Button
                                variant="ghost"
                                size="icon"
                                title="Delete Volume"
                                onClick={() =>
                                  setDeleteTarget({ type: 'volume', id: volume.id, name: volume.name })
                                }
                                className="h-8 w-8 text-gray-400 hover:text-red-400 hover:bg-red-500/10"
                              >
                                <Trash2 className="h-4 w-4" />
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
        </TabsContent>

        {/* ================================================================ */}
        {/* COMPOSE TAB                                                      */}
        {/* ================================================================ */}
        <TabsContent value="compose">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* Compose Editor */}
            <Card className="bg-gray-900 border-gray-800">
              <CardHeader>
                <CardTitle className="text-gray-100 text-lg flex items-center gap-2">
                  <FileCode size={20} />
                  Docker Compose Editor
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1.5">Stack Name</label>
                  <Input
                    placeholder="my-stack"
                    value={composeName}
                    onChange={(e) => setComposeName(e.target.value)}
                    className="bg-gray-800 border-gray-700 text-gray-200 placeholder:text-gray-500"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1.5">Compose YAML</label>
                  <textarea
                    value={composeContent}
                    onChange={(e) => setComposeContent(e.target.value)}
                    placeholder={`version: "3.8"\nservices:\n  web:\n    image: nginx:latest\n    ports:\n      - "8080:80"\n  db:\n    image: postgres:15\n    environment:\n      POSTGRES_PASSWORD: example`}
                    className="w-full h-72 rounded-md border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-200 font-mono placeholder:text-gray-600 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent resize-y"
                    spellCheck={false}
                  />
                </div>
                <div className="flex gap-3">
                  <Button
                    onClick={handleDeployCompose}
                    disabled={composeDeploying || !composeName.trim() || !composeContent.trim()}
                    className="bg-green-600 hover:bg-green-700 text-white"
                  >
                    {composeDeploying ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Play className="mr-2 h-4 w-4" />
                    )}
                    Deploy
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => {
                      setComposeName('');
                      setComposeContent('');
                    }}
                    className="border-gray-700 text-gray-300 hover:bg-gray-800"
                  >
                    Clear
                  </Button>
                </div>
              </CardContent>
            </Card>

            {/* Running Stacks */}
            <Card className="bg-gray-900 border-gray-800">
              <CardHeader>
                <CardTitle className="text-gray-100 text-lg flex items-center gap-2">
                  <Container size={20} />
                  Running Stacks
                </CardTitle>
              </CardHeader>
              <CardContent>
                {containers.filter((c) => c.name.includes('_')).length === 0 ? (
                  <div className="text-center py-12">
                    <FileCode className="mx-auto text-gray-600" size={48} />
                    <h3 className="mt-4 text-lg font-medium text-gray-400">No compose stacks</h3>
                    <p className="mt-2 text-gray-500 text-sm">
                      Deploy a compose stack using the editor
                    </p>
                  </div>
                ) : (
                  <div className="space-y-3">
                    {Array.from(
                      new Set(
                        containers
                          .filter((c) => c.name.includes('_'))
                          .map((c) => c.name.split('_')[0])
                      )
                    ).map((stackName) => {
                      const stackContainers = containers.filter(
                        (c) => c.name.startsWith(stackName + '_')
                      );
                      const allRunning = stackContainers.every(
                        (c) => c.state === 'running'
                      );
                      return (
                        <div
                          key={stackName}
                          className="flex items-center justify-between p-3 rounded-lg border border-gray-800 bg-gray-800/30"
                        >
                          <div className="flex items-center gap-3">
                            <FileCode size={18} className="text-gray-500" />
                            <div>
                              <p className="text-gray-200 font-medium">{stackName}</p>
                              <p className="text-xs text-gray-500">
                                {stackContainers.length} container{stackContainers.length !== 1 ? 's' : ''}
                              </p>
                            </div>
                          </div>
                          <div className="flex items-center gap-2">
                            <Badge variant={allRunning ? 'success' : 'warning'}>
                              {allRunning ? 'Running' : 'Partial'}
                            </Badge>
                            <Button
                              variant="ghost"
                              size="icon"
                              title="Stop Stack"
                              disabled={actionLoading === `compose-stop-${stackName}`}
                              onClick={() => handleStopCompose(stackName)}
                              className="h-8 w-8 text-gray-400 hover:text-red-400 hover:bg-red-500/10"
                            >
                              {actionLoading === `compose-stop-${stackName}` ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <Square className="h-4 w-4" />
                              )}
                            </Button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
}
