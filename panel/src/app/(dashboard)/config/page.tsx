'use client';

import { useState, useEffect } from 'react';
import {
  Globe, Feather, Code2, Database, Server, Shield, Radio,
  Lock, Clock, Settings, FileText, X
} from 'lucide-react';
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

const CARD = 'bg-white border border-gray-200 rounded-lg shadow-sm';
const CARD_HEADER = 'px-5 py-4 border-b border-gray-200';
const TH = 'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
const TD = 'px-4 py-3 text-sm text-gray-700';
const ROW = 'border-b border-gray-100 hover:bg-gray-50';
const BADGE = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium';
const FIELD =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none';
const LABEL = 'mb-1.5 block text-sm font-medium text-gray-700';
const BTN_PRIMARY =
  'inline-flex items-center gap-2 rounded-md bg-blue-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-1';
const BTN_SECONDARY =
  'inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-1';
const LINK_BTN =
  'rounded-md px-2 py-1 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500';

export default function ConfigPage() {
  const [snapshots, setSnapshots] = useState<ConfigSnapshot[]>([]);
  const [stats, setStats] = useState<ConfigStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
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
      setError('');
      const [snapshotsRes, statsRes] = await Promise.all([
        configApi.listSnapshots(filter),
        configApi.getStats(),
      ]);
      setSnapshots(Array.isArray(snapshotsRes?.data?.data) ? snapshotsRes.data.data : []);
      setStats(statsRes?.data ?? null);
    } catch (error) {
      console.error('Failed to load config data:', error);
      setError('Unable to load configuration snapshots. Please try again.');
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
      setError('Rollback failed. Please try again.');
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this snapshot?')) return;
    try {
      await configApi.deleteSnapshot(id);
      loadData();
    } catch (error) {
      console.error('Failed to delete snapshot:', error);
      setError('Unable to delete the snapshot. Please try again.');
    }
  };

  const handleViewDiff = async () => {
    if (!diffIds.id1 || !diffIds.id2) {
      setError('Please select two snapshots to compare.');
      return;
    }
    try {
      setError('');
      const res = await configApi.getDiff(diffIds.id1, diffIds.id2);
      setDiff(res?.data ?? null);
      setShowDiff(true);
    } catch (error) {
      console.error('Failed to get diff:', error);
      setError('Unable to compare the selected snapshots. Please try again.');
    }
  };

  const formatBytes = (bytes: number) => {
    if (!bytes || bytes <= 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return '-';
    const d = new Date(dateStr);
    if (Number.isNaN(d.getTime())) return '-';
    return d.toLocaleString();
  };

  const getConfigTypeIcon = (type: string) => {
    switch (type) {
      case 'nginx': return <Globe size={14} className="text-gray-500" />;
      case 'apache': return <Feather size={14} className="text-gray-500" />;
      case 'php': return <Code2 size={14} className="text-gray-500" />;
      case 'mysql': return <Database size={14} className="text-gray-500" />;
      case 'postgresql': return <Database size={14} className="text-gray-500" />;
      case 'redis': return <Server size={14} className="text-gray-500" />;
      case 'firewall': return <Shield size={14} className="text-gray-500" />;
      case 'dns': return <Radio size={14} className="text-gray-500" />;
      case 'ssl': return <Lock size={14} className="text-gray-500" />;
      case 'cron': return <Clock size={14} className="text-gray-500" />;
      case 'systemd': return <Settings size={14} className="text-gray-500" />;
      default: return <FileText size={14} className="text-gray-500" />;
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Configuration Rollback</h1>
          <p className="mt-1 text-sm text-gray-600">Track configuration snapshots, compare versions and roll back safely</p>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {/* Stats Cards */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div className={`${CARD} p-4`}>
            <div className="text-xs font-medium uppercase tracking-wide text-gray-500">Total Snapshots</div>
            <div className="mt-1 text-2xl font-semibold text-gray-900">{stats.total_snapshots ?? 0}</div>
          </div>
          <div className={`${CARD} p-4`}>
            <div className="text-xs font-medium uppercase tracking-wide text-gray-500">Active Configs</div>
            <div className="mt-1 text-2xl font-semibold text-emerald-600">{stats.active_configs ?? 0}</div>
          </div>
          <div className={`${CARD} p-4`}>
            <div className="text-xs font-medium uppercase tracking-wide text-gray-500">Storage Used</div>
            <div className="mt-1 text-2xl font-semibold text-gray-900">{formatBytes(stats.storage_used_bytes)}</div>
          </div>
          <div className={`${CARD} p-4`}>
            <div className="text-xs font-medium uppercase tracking-wide text-gray-500">Last Snapshot</div>
            <div className="mt-1 text-base font-semibold text-gray-900" suppressHydrationWarning>
              {stats.last_snapshot ? formatDate(stats.last_snapshot) : '-'}
            </div>
          </div>
        </div>
      )}

      {/* Filters */}
      <div className={CARD}>
        <div className={CARD_HEADER}>
          <h2 className="text-sm font-semibold text-gray-900">Filters</h2>
        </div>
        <div className="p-5">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label htmlFor="config-type" className={LABEL}>Config Type</label>
              <select
                id="config-type"
                value={filter.config_type}
                onChange={(e) => setFilter({ ...filter, config_type: e.target.value })}
                className={FIELD}
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
              <label htmlFor="config-name" className={LABEL}>Name</label>
              <input
                id="config-name"
                type="text"
                value={filter.name}
                onChange={(e) => setFilter({ ...filter, name: e.target.value })}
                placeholder="Search by name..."
                className={FIELD}
              />
            </div>
            <div>
              <label htmlFor="config-server" className={LABEL}>Server ID</label>
              <input
                id="config-server"
                type="text"
                value={filter.server_id}
                onChange={(e) => setFilter({ ...filter, server_id: e.target.value })}
                placeholder="Server ID..."
                className={FIELD}
              />
            </div>
          </div>
        </div>
      </div>

      {/* Diff Comparison */}
      <div className={CARD}>
        <div className={CARD_HEADER}>
          <h2 className="text-sm font-semibold text-gray-900">Compare Versions</h2>
        </div>
        <div className="p-5">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label htmlFor="diff-snapshot-1" className={LABEL}>Snapshot 1</label>
              <select
                id="diff-snapshot-1"
                value={diffIds.id1}
                onChange={(e) => setDiffIds({ ...diffIds, id1: e.target.value })}
                className={FIELD}
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
              <label htmlFor="diff-snapshot-2" className={LABEL}>Snapshot 2</label>
              <select
                id="diff-snapshot-2"
                value={diffIds.id2}
                onChange={(e) => setDiffIds({ ...diffIds, id2: e.target.value })}
                className={FIELD}
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
              <button type="button" onClick={handleViewDiff} className={BTN_PRIMARY}>
                Compare
              </button>
            </div>
          </div>

          {/* Diff Display */}
          {showDiff && diff && (
            <div className="mt-5 rounded-md border border-gray-200 p-4">
              <h3 className="mb-3 text-sm font-semibold text-gray-900">
                Diff: v{diff.old_version} → v{diff.new_version}
              </h3>
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div>
                  <h4 className="mb-2 text-sm font-medium text-red-700">Deletions ({diff.deletions?.length || 0})</h4>
                  <div className="max-h-60 overflow-y-auto rounded-md border border-red-200 bg-red-50 p-3 font-mono text-sm">
                    {(diff.deletions ?? []).map((line, i) => (
                      <div key={i} className="text-red-700">- {line}</div>
                    ))}
                  </div>
                </div>
                <div>
                  <h4 className="mb-2 text-sm font-medium text-emerald-700">Additions ({diff.additions?.length || 0})</h4>
                  <div className="max-h-60 overflow-y-auto rounded-md border border-emerald-200 bg-emerald-50 p-3 font-mono text-sm">
                    {(diff.additions ?? []).map((line, i) => (
                      <div key={i} className="text-emerald-700">+ {line}</div>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Snapshots Table */}
      <div className={`${CARD} overflow-hidden`}>
        <div className={CARD_HEADER}>
          <h2 className="text-sm font-semibold text-gray-900">Snapshots</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full">
            <thead className="bg-gray-50 border-b border-gray-200">
              <tr>
                <th className={TH}>Type</th>
                <th className={TH}>Name</th>
                <th className={TH}>Version</th>
                <th className={TH}>Status</th>
                <th className={TH}>Created</th>
                <th className={`${TH} text-right`}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr>
                  <td colSpan={6} className="px-4 py-10 text-center text-sm text-gray-500">
                    Loading…
                  </td>
                </tr>
              ) : snapshots.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-10 text-center text-sm text-gray-500">
                    No snapshots found
                  </td>
                </tr>
              ) : (
                snapshots.map((snapshot) => (
                  <tr key={snapshot.id} className={ROW}>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        {getConfigTypeIcon(snapshot.config_type)}
                        <span className="text-sm text-gray-700">{snapshot.config_type || '—'}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="text-sm font-medium text-gray-900">{snapshot.name}</div>
                      {snapshot.description && (
                        <div className="max-w-xs truncate text-sm text-gray-500">{snapshot.description}</div>
                      )}
                    </td>
                    <td className={TD}>v{snapshot.version ?? 0}</td>
                    <td className="px-4 py-3">
                      {snapshot.is_active ? (
                        <span className={`${BADGE} bg-emerald-50 text-emerald-700`}>Active</span>
                      ) : (
                        <span className={`${BADGE} bg-gray-100 text-gray-700`}>Inactive</span>
                      )}
                      {snapshot.is_automatic && (
                        <span className={`${BADGE} ml-2 bg-blue-50 text-blue-700`}>Auto</span>
                      )}
                    </td>
                    <td className={TD} suppressHydrationWarning>{formatDate(snapshot.created_at)}</td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex justify-end gap-2">
                        {!snapshot.is_active && (
                          <button
                            type="button"
                            onClick={() => handleRollback(snapshot.id)}
                            className={`${LINK_BTN} text-blue-700 hover:bg-blue-50`}
                          >
                            Rollback
                          </button>
                        )}
                        <button
                          type="button"
                          onClick={() => setSelectedSnapshot(snapshot)}
                          className={`${LINK_BTN} text-gray-700 hover:bg-gray-100`}
                        >
                          View
                        </button>
                        {!snapshot.is_active && (
                          <button
                            type="button"
                            onClick={() => handleDelete(snapshot.id)}
                            className={`${LINK_BTN} text-red-700 hover:bg-red-50`}
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
      </div>

      {/* Snapshot Detail Modal */}
      {selectedSnapshot && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40 p-4">
          <div className="max-h-[80vh] w-full max-w-4xl overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg">
            <div className="flex items-center justify-between gap-4 border-b border-gray-200 px-5 py-4">
              <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                {getConfigTypeIcon(selectedSnapshot.config_type)} {selectedSnapshot.name} v{selectedSnapshot.version}
              </h2>
              <button
                type="button"
                aria-label="Close snapshot detail"
                onClick={() => setSelectedSnapshot(null)}
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
              >
                <X size={18} />
              </button>
            </div>
            <div className="p-5">
              <div className="mb-4 space-y-1">
                <div className="text-sm text-gray-600">Path: {selectedSnapshot.path || '—'}</div>
                <div className="text-sm text-gray-600">Checksum: {selectedSnapshot.checksum || '—'}</div>
                <div className="text-sm text-gray-600" suppressHydrationWarning>Created: {formatDate(selectedSnapshot.created_at)}</div>
              </div>
              <div className="rounded-md border border-gray-200 bg-gray-50 p-4">
                <pre className="overflow-x-auto whitespace-pre-wrap font-mono text-sm text-gray-700">
                  {selectedSnapshot.content}
                </pre>
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              {!selectedSnapshot.is_active && (
                <button
                  type="button"
                  onClick={() => {
                    handleRollback(selectedSnapshot.id);
                    setSelectedSnapshot(null);
                  }}
                  className={BTN_PRIMARY}
                >
                  Rollback to this version
                </button>
              )}
              <button type="button" onClick={() => setSelectedSnapshot(null)} className={BTN_SECONDARY}>
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Config Types Distribution */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div className={CARD}>
            <div className={CARD_HEADER}>
              <h2 className="text-sm font-semibold text-gray-900">Snapshots by Type</h2>
            </div>
            <div className="p-5">
              {Object.keys(stats.by_type ?? {}).length === 0 ? (
                <p className="py-4 text-center text-sm text-gray-500">No data available</p>
              ) : (
                <div className="space-y-3">
                  {Object.entries(stats.by_type ?? {}).map(([type, count]) => (
                    <div key={type} className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        {getConfigTypeIcon(type)}
                        <span className="text-sm text-gray-700">{type}</span>
                      </div>
                      <span className="text-sm font-medium text-gray-900">{count}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
          <div className={CARD}>
            <div className={CARD_HEADER}>
              <h2 className="text-sm font-semibold text-gray-900">Snapshots by Server</h2>
            </div>
            <div className="p-5">
              {Object.keys(stats.by_server ?? {}).length === 0 ? (
                <p className="py-4 text-center text-sm text-gray-500">No data available</p>
              ) : (
                <div className="space-y-3">
                  {Object.entries(stats.by_server ?? {}).map(([server, count]) => (
                    <div key={server} className="flex items-center justify-between">
                      <span className="text-sm text-gray-700">{server}</span>
                      <span className="text-sm font-medium text-gray-900">{count}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
