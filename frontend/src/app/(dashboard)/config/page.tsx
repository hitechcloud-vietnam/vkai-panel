'use client';

import { useState, useEffect } from 'react';
import { configApi } from '@/services/api';

interface ConfigSnapshot {
  id: string;
  config_type: string;
  name: string;
  path: string;
  content: string;
  checksum: string;
  version: number;
  is_active: boolean;
  is_automatic: boolean;
  description: string;
  created_at: string;
}

interface ConfigStats {
  total_snapshots: number;
  by_type: Record<string, number>;
  by_server: Record<string, number>;
  active_configs: number;
  last_snapshot: string;
  storage_used_bytes: number;
}

interface ConfigDiff {
  old_version: number;
  new_version: number;
  old_content: string;
  new_content: string;
  additions: string[];
  deletions: string[];
  changes: string[];
}

export default function ConfigPage() {
  const [snapshots, setSnapshots] = useState<ConfigSnapshot[]>([]);
  const [stats, setStats] = useState<ConfigStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [selectedSnapshot, setSelectedSnapshot] = useState<ConfigSnapshot | null>(null);
  const [diff, setDiff] = useState<ConfigDiff | null>(null);
  const [showDiff, setShowDiff] = useState(false);
  const [diffIds, setDiffIds] = useState({ id1: '', id2: '' });
  const [filter, setFilter] = useState({
    config_type: '',
    name: '',
    server_id: '',
  });

  useEffect(() => {
    loadData();
  }, [filter]);

  const loadData = async () => {
    try {
      setLoading(true);
      const [snapshotsRes, statsRes] = await Promise.all([
        configApi.listSnapshots(filter),
        configApi.getStats(),
      ]);
      setSnapshots(snapshotsRes.data.data || []);
      setStats(statsRes.data);
    } catch (error) {
      console.error('Failed to load config data:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleRollback = async (snapshotId: string) => {
    if (!confirm('Are you sure you want to rollback to this version?')) return;
    try {
      await configApi.rollback({
        snapshot_id: snapshotId,
        reason: 'Manual rollback',
      });
      loadData();
      alert('Rollback successful!');
    } catch (error) {
      console.error('Failed to rollback:', error);
      alert('Rollback failed');
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this snapshot?')) return;
    try {
      await configApi.deleteSnapshot(id);
      loadData();
    } catch (error) {
      console.error('Failed to delete snapshot:', error);
    }
  };

  const handleViewDiff = async () => {
    if (!diffIds.id1 || !diffIds.id2) {
      alert('Please select two snapshots to compare');
      return;
    }
    try {
      const res = await configApi.getDiff(diffIds.id1, diffIds.id2);
      setDiff(res.data);
      setShowDiff(true);
    } catch (error) {
      console.error('Failed to get diff:', error);
    }
  };

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return '-';
    return new Date(dateStr).toLocaleString();
  };

  const getConfigTypeIcon = (type: string) => {
    switch (type) {
      case 'nginx': return '🌐';
      case 'apache': return '🪶';
      case 'php': return '🐘';
      case 'mysql': return '🐬';
      case 'postgresql': return '🐘';
      case 'redis': return '🔴';
      case 'firewall': return '🛡️';
      case 'dns': return '📡';
      case 'ssl': return '🔒';
      case 'cron': return '⏰';
      case 'systemd': return '⚙️';
      default: return '📄';
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">Configuration Rollback</h1>
      </div>

      {/* Stats Cards */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div className="bg-white rounded-lg shadow p-4">
            <div className="text-sm text-gray-500">Total Snapshots</div>
            <div className="text-2xl font-bold">{stats.total_snapshots}</div>
          </div>
          <div className="bg-white rounded-lg shadow p-4">
            <div className="text-sm text-gray-500">Active Configs</div>
            <div className="text-2xl font-bold text-green-600">{stats.active_configs}</div>
          </div>
          <div className="bg-white rounded-lg shadow p-4">
            <div className="text-sm text-gray-500">Storage Used</div>
            <div className="text-2xl font-bold">{formatBytes(stats.storage_used_bytes)}</div>
          </div>
          <div className="bg-white rounded-lg shadow p-4">
            <div className="text-sm text-gray-500">Last Snapshot</div>
            <div className="text-lg font-bold">{stats.last_snapshot ? formatDate(stats.last_snapshot) : '-'}</div>
          </div>
        </div>
      )}

      {/* Filters */}
      <div className="bg-white rounded-lg shadow p-4">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Config Type</label>
            <select
              value={filter.config_type}
              onChange={(e) => setFilter({ ...filter, config_type: e.target.value })}
              className="w-full border rounded-lg px-3 py-2"
            >
              <option value="">All Types</option>
              <option value="nginx">Nginx</option>
              <option value="apache">Apache</option>
              <option value="php">PHP</option>
              <option value="mysql">MySQL</option>
              <option value="postgresql">PostgreSQL</option>
              <option value="redis">Redis</option>
              <option value="firewall">Firewall</option>
              <option value="dns">DNS</option>
              <option value="ssl">SSL</option>
              <option value="cron">Cron</option>
              <option value="systemd">Systemd</option>
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
            <input
              type="text"
              value={filter.name}
              onChange={(e) => setFilter({ ...filter, name: e.target.value })}
              placeholder="Search by name..."
              className="w-full border rounded-lg px-3 py-2"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Server ID</label>
            <input
              type="text"
              value={filter.server_id}
              onChange={(e) => setFilter({ ...filter, server_id: e.target.value })}
              placeholder="Server ID..."
              className="w-full border rounded-lg px-3 py-2"
            />
          </div>
        </div>
      </div>

      {/* Diff Comparison */}
      <div className="bg-white rounded-lg shadow p-4">
        <h3 className="text-lg font-semibold mb-4">Compare Versions</h3>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Snapshot 1</label>
            <select
              value={diffIds.id1}
              onChange={(e) => setDiffIds({ ...diffIds, id1: e.target.value })}
              className="w-full border rounded-lg px-3 py-2"
            >
              <option value="">Select snapshot...</option>
              {snapshots.map((s) => (
                <option key={s.id} value={s.id}>
                  v{s.version} - {s.name} ({s.config_type})
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Snapshot 2</label>
            <select
              value={diffIds.id2}
              onChange={(e) => setDiffIds({ ...diffIds, id2: e.target.value })}
              className="w-full border rounded-lg px-3 py-2"
            >
              <option value="">Select snapshot...</option>
              {snapshots.map((s) => (
                <option key={s.id} value={s.id}>
                  v{s.version} - {s.name} ({s.config_type})
                </option>
              ))}
            </select>
          </div>
          <div className="flex items-end">
            <button
              onClick={handleViewDiff}
              className="bg-blue-600 text-white px-4 py-2 rounded-lg hover:bg-blue-700"
            >
              Compare
            </button>
          </div>
        </div>

        {/* Diff Display */}
        {showDiff && diff && (
          <div className="mt-4 border rounded-lg p-4">
            <h4 className="font-semibold mb-2">
              Diff: v{diff.old_version} → v{diff.new_version}
            </h4>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <h5 className="text-sm font-medium text-red-600 mb-2">Deletions ({diff.deletions?.length || 0})</h5>
                <div className="bg-red-50 p-3 rounded text-sm font-mono max-h-60 overflow-y-auto">
                  {diff.deletions?.map((line, i) => (
                    <div key={i} className="text-red-700">- {line}</div>
                  ))}
                </div>
              </div>
              <div>
                <h5 className="text-sm font-medium text-green-600 mb-2">Additions ({diff.additions?.length || 0})</h5>
                <div className="bg-green-50 p-3 rounded text-sm font-mono max-h-60 overflow-y-auto">
                  {diff.additions?.map((line, i) => (
                    <div key={i} className="text-green-700">+ {line}</div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Snapshots Table */}
      <div className="bg-white rounded-lg shadow overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Type</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Version</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Created</th>
              <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200">
            {loading ? (
              <tr>
                <td colSpan={6} className="px-6 py-4 text-center text-gray-500">
                  Loading...
                </td>
              </tr>
            ) : snapshots.length === 0 ? (
              <tr>
                <td colSpan={6} className="px-6 py-4 text-center text-gray-500">
                  No snapshots found
                </td>
              </tr>
            ) : (
              snapshots.map((snapshot) => (
                <tr key={snapshot.id} className="hover:bg-gray-50">
                  <td className="px-6 py-4">
                    <div className="flex items-center">
                      <span className="mr-2">{getConfigTypeIcon(snapshot.config_type)}</span>
                      <span>{snapshot.config_type}</span>
                    </div>
                  </td>
                  <td className="px-6 py-4">
                    <div className="font-medium">{snapshot.name}</div>
                    {snapshot.description && (
                      <div className="text-sm text-gray-500 truncate max-w-xs">{snapshot.description}</div>
                    )}
                  </td>
                  <td className="px-6 py-4 text-sm text-gray-500">v{snapshot.version}</td>
                  <td className="px-6 py-4">
                    {snapshot.is_active ? (
                      <span className="px-2 py-1 rounded-full text-xs font-medium bg-green-100 text-green-800">
                        Active
                      </span>
                    ) : (
                      <span className="px-2 py-1 rounded-full text-xs font-medium bg-gray-100 text-gray-800">
                        Inactive
                      </span>
                    )}
                    {snapshot.is_automatic && (
                      <span className="ml-2 px-2 py-1 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
                        Auto
                      </span>
                    )}
                  </td>
                  <td className="px-6 py-4 text-sm text-gray-500">{formatDate(snapshot.created_at)}</td>
                  <td className="px-6 py-4 text-right">
                    <div className="flex justify-end space-x-2">
                      {!snapshot.is_active && (
                        <button
                          onClick={() => handleRollback(snapshot.id)}
                          className="text-blue-600 hover:text-blue-900 text-sm"
                        >
                          Rollback
                        </button>
                      )}
                      <button
                        onClick={() => setSelectedSnapshot(snapshot)}
                        className="text-gray-600 hover:text-gray-900 text-sm"
                      >
                        View
                      </button>
                      {!snapshot.is_active && (
                        <button
                          onClick={() => handleDelete(snapshot.id)}
                          className="text-red-600 hover:text-red-900 text-sm"
                        >
                          Delete
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Snapshot Detail Modal */}
      {selectedSnapshot && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 max-w-4xl w-full max-h-[80vh] overflow-y-auto">
            <div className="flex justify-between items-center mb-4">
              <h2 className="text-xl font-bold">
                {getConfigTypeIcon(selectedSnapshot.config_type)} {selectedSnapshot.name} v{selectedSnapshot.version}
              </h2>
              <button
                onClick={() => setSelectedSnapshot(null)}
                className="text-gray-500 hover:text-gray-700"
              >
                ✕
              </button>
            </div>
            <div className="mb-4">
              <div className="text-sm text-gray-500">Path: {selectedSnapshot.path}</div>
              <div className="text-sm text-gray-500">Checksum: {selectedSnapshot.checksum}</div>
              <div className="text-sm text-gray-500">Created: {formatDate(selectedSnapshot.created_at)}</div>
            </div>
            <div className="bg-gray-50 p-4 rounded-lg">
              <pre className="text-sm font-mono whitespace-pre-wrap overflow-x-auto">
                {selectedSnapshot.content}
              </pre>
            </div>
            <div className="mt-4 flex justify-end space-x-2">
              {!selectedSnapshot.is_active && (
                <button
                  onClick={() => {
                    handleRollback(selectedSnapshot.id);
                    setSelectedSnapshot(null);
                  }}
                  className="bg-blue-600 text-white px-4 py-2 rounded-lg hover:bg-blue-700"
                >
                  Rollback to this version
                </button>
              )}
              <button
                onClick={() => setSelectedSnapshot(null)}
                className="bg-gray-300 text-gray-700 px-4 py-2 rounded-lg hover:bg-gray-400"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Config Types Distribution */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div className="bg-white rounded-lg shadow p-6">
            <h3 className="text-lg font-semibold mb-4">Snapshots by Type</h3>
            <div className="space-y-3">
              {Object.entries(stats.by_type).map(([type, count]) => (
                <div key={type} className="flex items-center justify-between">
                  <div className="flex items-center">
                    <span className="mr-2">{getConfigTypeIcon(type)}</span>
                    <span>{type}</span>
                  </div>
                  <span className="font-medium">{count}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="bg-white rounded-lg shadow p-6">
            <h3 className="text-lg font-semibold mb-4">Snapshots by Server</h3>
            <div className="space-y-3">
              {Object.entries(stats.by_server).map(([server, count]) => (
                <div key={server} className="flex items-center justify-between">
                  <span>{server}</span>
                  <span className="font-medium">{count}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
