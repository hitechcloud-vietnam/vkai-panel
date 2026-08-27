'use client';

import { useState, useEffect } from 'react';
import { Shield, Plus, Scan, RefreshCw, AlertTriangle, CheckCircle, XCircle, Eye, Trash2, FileCheck, History, Lock, Unlock } from 'lucide-react';

interface ProtectedPath {
  id: string;
  path: string;
  path_type: string;
  recursive: boolean;
  algorithm: string;
  is_enabled: boolean;
  alert_on_change: boolean;
  alert_on_delete: boolean;
  alert_on_create: boolean;
  ignore_patterns: string[];
  description: string;
  file_count: number;
  last_scan_at: string | null;
  last_alert_at: string | null;
  created_at: string;
}

interface TamperAlert {
  id: string;
  protected_id: string;
  file_path: string;
  alert_type: string;
  severity: string;
  old_checksum: string;
  new_checksum: string;
  old_size: number;
  new_size: number;
  is_resolved: boolean;
  resolved_by: string;
  resolved_at: string | null;
  notes: string;
  created_at: string;
}

interface ScanResult {
  id: string;
  protected_id: string;
  status: string;
  total_files: number;
  scanned_files: number;
  violations: number;
  new_files: number;
  deleted_files: number;
  modified_files: number;
  duration: number;
  scan_log: string;
  created_at: string;
}

interface TamperStats {
  protected_paths: number;
  enabled_paths: number;
  total_files: number;
  active_alerts: number;
  resolved_alerts: number;
  alerts_today: number;
  last_scan_at: string | null;
  total_scans: number;
  clean_scans: number;
  violation_scans: number;
}

export default function TamperProofPage() {
  const [stats, setStats] = useState<TamperStats | null>(null);
  const [paths, setPaths] = useState<ProtectedPath[]>([]);
  const [alerts, setAlerts] = useState<TamperAlert[]>([]);
  const [scanResults, setScanResults] = useState<ScanResult[]>([]);
  const [activeTab, setActiveTab] = useState<'paths' | 'alerts' | 'scans' | 'audit'>('paths');
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [scanning, setScanning] = useState<string | null>(null);
  const [auditLogs, setAuditLogs] = useState<any[]>([]);
  const [newPath, setNewPath] = useState({
    path: '',
    path_type: 'file',
    recursive: false,
    algorithm: 'sha256',
    alert_on_change: true,
    alert_on_delete: true,
    alert_on_create: false,
    ignore_patterns: '',
    description: '',
  });

  const getToken = () => {
    if (typeof window !== 'undefined') {
      const authStorage = localStorage.getItem('auth-storage');
      if (authStorage) {
        try {
          const parsed = JSON.parse(authStorage);
          return parsed?.state?.token || '';
        } catch { return ''; }
      }
    }
    return '';
  };

  const apiCall = async (url: string, options: RequestInit = {}) => {
    const token = getToken();
    const res = await fetch(`/api${url}`, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`,
        ...options.headers,
      },
    });
    if (!res.ok) throw new Error(`API error: ${res.status}`);
    return res.json();
  };

  const fetchAll = async () => {
    setLoading(true);
    try {
      const [statsRes, pathsRes, alertsRes, scansRes, auditRes] = await Promise.all([
        apiCall('/v1/tamper-proof/stats').catch(() => ({ stats: null })),
        apiCall('/v1/tamper-proof/paths').catch(() => ({ paths: [] })),
        apiCall('/v1/tamper-proof/alerts').catch(() => ({ alerts: [] })),
        apiCall('/v1/tamper-proof/scan-results?limit=30').catch(() => ({ results: [] })),
        apiCall('/v1/tamper-proof/audit-logs?limit=50').catch(() => ({ logs: [] })),
      ]);
      setStats(statsRes.stats);
      setPaths(pathsRes.paths || []);
      setAlerts(alertsRes.alerts || []);
      setScanResults(scansRes.results || []);
      setAuditLogs(auditRes.logs || []);
    } catch (err) {
      console.error('Failed to fetch data:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchAll(); }, []);

  const handleCreatePath = async () => {
    try {
      await apiCall('/v1/tamper-proof/paths', {
        method: 'POST',
        body: JSON.stringify({
          ...newPath,
          ignore_patterns: newPath.ignore_patterns.split(',').map(s => s.trim()).filter(Boolean),
        }),
      });
      setShowCreate(false);
      setNewPath({ path: '', path_type: 'file', recursive: false, algorithm: 'sha256', alert_on_change: true, alert_on_delete: true, alert_on_create: false, ignore_patterns: '', description: '' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to create path:', err);
    }
  };

  const handleScan = async (id: string) => {
    setScanning(id);
    try {
      await apiCall(`/v1/tamper-proof/paths/${id}/scan`, { method: 'POST' });
      await fetchAll();
    } catch (err) {
      console.error('Scan failed:', err);
    } finally {
      setScanning(null);
    }
  };

  const handleScanAll = async () => {
    setScanning('all');
    try {
      await apiCall('/v1/tamper-proof/scan-all', { method: 'POST' });
      await fetchAll();
    } catch (err) {
      console.error('Scan all failed:', err);
    } finally {
      setScanning(null);
    }
  };

  const handleDeletePath = async (id: string) => {
    if (!confirm('Delete this protected path and all its baselines?')) return;
    try {
      await apiCall(`/v1/tamper-proof/paths/${id}`, { method: 'DELETE' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to delete:', err);
    }
  };

  const handleResolveAlert = async (id: string) => {
    try {
      await apiCall(`/v1/tamper-proof/alerts/${id}/resolve`, {
        method: 'POST',
        body: JSON.stringify({ notes: 'Resolved by admin' }),
      });
      await fetchAll();
    } catch (err) {
      console.error('Failed to resolve:', err);
    }
  };

  const handleRefreshBaseline = async (id: string) => {
    if (!confirm('This will re-scan and update all checksums. Continue?')) return;
    try {
      await apiCall(`/v1/tamper-proof/paths/${id}/baselines/refresh`, { method: 'POST' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to refresh:', err);
    }
  };

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case 'critical': return 'text-red-400 bg-red-400/10';
      case 'high': return 'text-orange-400 bg-orange-400/10';
      case 'medium': return 'text-yellow-400 bg-yellow-400/10';
      case 'low': return 'text-blue-400 bg-blue-400/10';
      default: return 'text-gray-400 bg-gray-400/10';
    }
  };

  const getAlertTypeIcon = (type: string) => {
    switch (type) {
      case 'modified': return <AlertTriangle size={16} className="text-orange-400" />;
      case 'deleted': return <XCircle size={16} className="text-red-400" />;
      case 'created': return <Plus size={16} className="text-blue-400" />;
      default: return <AlertTriangle size={16} className="text-yellow-400" />;
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="animate-spin text-blue-400" size={32} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Tamper Proof for Enterprise</h1>
          <p className="text-gray-400 mt-1">File integrity monitoring, intrusion detection & audit trail</p>
        </div>
        <div className="flex gap-3">
          <button onClick={fetchAll} className="flex items-center gap-2 px-4 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg transition-colors">
            <RefreshCw size={16} /> Refresh
          </button>
          <button onClick={handleScanAll} disabled={scanning !== null}
            className="flex items-center gap-2 px-4 py-2 bg-green-600 hover:bg-green-700 text-white rounded-lg transition-colors disabled:opacity-50">
            <Scan size={16} /> {scanning === 'all' ? 'Scanning...' : 'Scan All'}
          </button>
          <button onClick={() => setShowCreate(true)} className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors">
            <Plus size={16} /> Add Path
          </button>
        </div>
      </div>

      {/* Stats */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
          {[
            { label: 'Protected Paths', value: stats.protected_paths, icon: <Shield size={20} />, color: 'text-blue-400' },
            { label: 'Monitored Files', value: stats.total_files, icon: <FileCheck size={20} />, color: 'text-green-400' },
            { label: 'Active Alerts', value: stats.active_alerts, icon: <AlertTriangle size={20} />, color: stats.active_alerts > 0 ? 'text-red-400' : 'text-green-400' },
            { label: 'Clean Scans', value: stats.clean_scans, icon: <CheckCircle size={20} />, color: 'text-purple-400' },
            { label: 'Total Scans', value: stats.total_scans, icon: <History size={20} />, color: 'text-gray-400' },
          ].map((stat, i) => (
            <div key={i} className="bg-gray-800 rounded-xl p-4 border border-gray-700">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-gray-400 text-sm">{stat.label}</p>
                  <p className={`text-2xl font-bold mt-1 ${stat.color}`}>{stat.value}</p>
                </div>
                <div className={`${stat.color} opacity-60`}>{stat.icon}</div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-1 bg-gray-800 rounded-lg p-1 w-fit">
        {(['paths', 'alerts', 'scans', 'audit'] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${activeTab === tab ? 'bg-blue-600 text-white' : 'text-gray-400 hover:text-white'}`}
          >
            {tab === 'paths' ? 'Protected Paths' : tab === 'alerts' ? `Alerts${stats?.active_alerts ? ` (${stats.active_alerts})` : ''}` : tab === 'scans' ? 'Scan Results' : 'Audit Log'}
          </button>
        ))}
      </div>

      {/* Protected Paths Tab */}
      {activeTab === 'paths' && (
        <div className="bg-gray-800 rounded-xl border border-gray-700">
          <div className="p-4 border-b border-gray-700">
            <h2 className="text-lg font-semibold text-white">Protected Paths</h2>
          </div>
          <div className="divide-y divide-gray-700">
            {paths.length === 0 ? (
              <div className="p-8 text-center text-gray-400">No protected paths configured. Add a file or directory to monitor.</div>
            ) : (
              paths.map((p) => (
                <div key={p.id} className="p-4 hover:bg-gray-750">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-4">
                      <div className={`p-2 rounded-lg ${p.is_enabled ? 'bg-green-600/20' : 'bg-gray-600/20'}`}>
                        {p.is_enabled ? <Lock size={16} className="text-green-400" /> : <Unlock size={16} className="text-gray-400" />}
                      </div>
                      <div>
                        <div className="flex items-center gap-2">
                          <p className="text-white font-mono text-sm">{p.path}</p>
                          <span className="px-2 py-0.5 bg-gray-700 text-gray-300 text-xs rounded-full">{p.path_type}</span>
                          <span className="px-2 py-0.5 bg-blue-600/20 text-blue-400 text-xs rounded-full">{p.algorithm}</span>
                          {p.recursive && <span className="px-2 py-0.5 bg-purple-600/20 text-purple-400 text-xs rounded-full">recursive</span>}
                        </div>
                        <div className="flex items-center gap-4 mt-1 text-xs text-gray-500">
                          <span>Files: {p.file_count}</span>
                          <span>Alerts: {p.alert_on_change ? 'change' : ''} {p.alert_on_delete ? 'delete' : ''} {p.alert_on_create ? 'create' : ''}</span>
                          {p.last_scan_at && <span>Last scan: {new Date(p.last_scan_at).toLocaleString()}</span>}
                        </div>
                        {p.description && <p className="text-gray-400 text-xs mt-1">{p.description}</p>}
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <button
                        onClick={() => handleScan(p.id)}
                        disabled={scanning === p.id}
                        className="p-2 text-gray-400 hover:text-green-400 transition-colors disabled:opacity-50"
                        title="Scan Now"
                      >
                        <Scan size={16} />
                      </button>
                      <button
                        onClick={() => handleRefreshBaseline(p.id)}
                        className="p-2 text-gray-400 hover:text-blue-400 transition-colors"
                        title="Refresh Baseline"
                      >
                        <RefreshCw size={16} />
                      </button>
                      <button
                        onClick={() => handleDeletePath(p.id)}
                        className="p-2 text-gray-400 hover:text-red-400 transition-colors"
                        title="Delete"
                      >
                        <Trash2 size={16} />
                      </button>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* Alerts Tab */}
      {activeTab === 'alerts' && (
        <div className="bg-gray-800 rounded-xl border border-gray-700">
          <div className="p-4 border-b border-gray-700">
            <h2 className="text-lg font-semibold text-white">Integrity Alerts</h2>
          </div>
          <div className="divide-y divide-gray-700">
            {alerts.length === 0 ? (
              <div className="p-8 text-center text-gray-400">No alerts. All files are clean.</div>
            ) : (
              alerts.map((alert) => (
                <div key={alert.id} className={`p-4 ${alert.is_resolved ? 'opacity-60' : ''}`}>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-4">
                      {getAlertTypeIcon(alert.alert_type)}
                      <div>
                        <div className="flex items-center gap-2">
                          <p className="text-white font-mono text-sm">{alert.file_path}</p>
                          <span className={`px-2 py-0.5 rounded-full text-xs ${getSeverityColor(alert.severity)}`}>{alert.severity}</span>
                          <span className="px-2 py-0.5 bg-gray-700 text-gray-300 text-xs rounded-full">{alert.alert_type}</span>
                          {alert.is_resolved && <span className="px-2 py-0.5 bg-green-600/20 text-green-400 text-xs rounded-full">resolved</span>}
                        </div>
                        <div className="flex items-center gap-4 mt-1 text-xs text-gray-500">
                          {alert.old_checksum && <span>Old: {alert.old_checksum.substring(0, 16)}...</span>}
                          {alert.new_checksum && <span>New: {alert.new_checksum.substring(0, 16)}...</span>}
                          <span>{new Date(alert.created_at).toLocaleString()}</span>
                        </div>
                      </div>
                    </div>
                    {!alert.is_resolved && (
                      <button
                        onClick={() => handleResolveAlert(alert.id)}
                        className="px-3 py-1 bg-green-600 hover:bg-green-700 text-white text-sm rounded-lg transition-colors"
                      >
                        Resolve
                      </button>
                    )}
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* Scan Results Tab */}
      {activeTab === 'scans' && (
        <div className="bg-gray-800 rounded-xl border border-gray-700">
          <div className="p-4 border-b border-gray-700">
            <h2 className="text-lg font-semibold text-white">Scan History</h2>
          </div>
          <div className="divide-y divide-gray-700">
            {scanResults.length === 0 ? (
              <div className="p-8 text-center text-gray-400">No scan results yet.</div>
            ) : (
              scanResults.map((scan) => (
                <div key={scan.id} className="p-4">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-4">
                      {scan.status === 'clean' ? <CheckCircle size={16} className="text-green-400" /> :
                       scan.status === 'violations_found' ? <AlertTriangle size={16} className="text-red-400" /> :
                       <XCircle size={16} className="text-gray-400" />}
                      <div>
                        <div className="flex items-center gap-2">
                          <span className={`text-sm font-medium ${scan.status === 'clean' ? 'text-green-400' : 'text-red-400'}`}>
                            {scan.status === 'clean' ? 'Clean' : scan.status === 'violations_found' ? 'Violations Found' : 'Error'}
                          </span>
                        </div>
                        <div className="flex items-center gap-4 mt-1 text-xs text-gray-500">
                          <span>Files: {scan.scanned_files}/{scan.total_files}</span>
                          {scan.violations > 0 && <span className="text-red-400">Violations: {scan.violations}</span>}
                          {scan.new_files > 0 && <span className="text-blue-400">New: {scan.new_files}</span>}
                          {scan.modified_files > 0 && <span className="text-orange-400">Modified: {scan.modified_files}</span>}
                          {scan.deleted_files > 0 && <span className="text-red-400">Deleted: {scan.deleted_files}</span>}
                          <span>Duration: {scan.duration}ms</span>
                          <span>{new Date(scan.created_at).toLocaleString()}</span>
                        </div>
                        {scan.scan_log && <pre className="text-gray-400 text-xs mt-1 font-mono">{scan.scan_log.substring(0, 200)}</pre>}
                      </div>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* Audit Log Tab */}
      {activeTab === 'audit' && (
        <div className="bg-gray-800 rounded-xl border border-gray-700">
          <div className="p-4 border-b border-gray-700">
            <h2 className="text-lg font-semibold text-white">Audit Trail</h2>
          </div>
          <div className="divide-y divide-gray-700">
            {auditLogs.length === 0 ? (
              <div className="p-8 text-center text-gray-400">No audit entries yet.</div>
            ) : (
              auditLogs.map((log) => (
                <div key={log.id} className="p-4 flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <History size={16} className="text-gray-400" />
                    <div>
                      <p className="text-white text-sm">{log.action}: <span className="text-gray-400">{log.target}</span></p>
                      {log.details && <p className="text-gray-500 text-xs mt-1">{log.details}</p>}
                    </div>
                  </div>
                  <div className="text-right">
                    <p className="text-gray-400 text-xs">{log.username}</p>
                    <p className="text-gray-500 text-xs">{new Date(log.created_at).toLocaleString()}</p>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* Create Path Modal */}
      {showCreate && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => setShowCreate(false)}>
          <div className="bg-gray-800 rounded-xl w-full max-w-lg border border-gray-700 max-h-[80vh] overflow-y-auto" onClick={(e) => e.stopPropagation()}>
            <div className="p-6 border-b border-gray-700">
              <h3 className="text-xl font-bold text-white">Add Protected Path</h3>
            </div>
            <div className="p-6 space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Path</label>
                <input type="text" value={newPath.path} onChange={(e) => setNewPath({ ...newPath, path: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500 font-mono"
                  placeholder="/etc/nginx/nginx.conf" />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1">Type</label>
                  <select value={newPath.path_type} onChange={(e) => setNewPath({ ...newPath, path_type: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500">
                    <option value="file">File</option>
                    <option value="directory">Directory</option>
                  </select>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1">Algorithm</label>
                  <select value={newPath.algorithm} onChange={(e) => setNewPath({ ...newPath, algorithm: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500">
                    <option value="sha256">SHA-256</option>
                    <option value="sha512">SHA-512</option>
                    <option value="md5">MD5</option>
                  </select>
                </div>
              </div>
              <div className="flex items-center gap-4">
                <label className="flex items-center gap-2 text-sm text-gray-300">
                  <input type="checkbox" checked={newPath.recursive} onChange={(e) => setNewPath({ ...newPath, recursive: e.target.checked })}
                    className="rounded bg-gray-700 border-gray-600" />
                  Recursive (directories)
                </label>
              </div>
              <div className="space-y-2">
                <p className="text-sm font-medium text-gray-300">Alert On:</p>
                <div className="flex items-center gap-4">
                  <label className="flex items-center gap-2 text-sm text-gray-300">
                    <input type="checkbox" checked={newPath.alert_on_change} onChange={(e) => setNewPath({ ...newPath, alert_on_change: e.target.checked })}
                      className="rounded bg-gray-700 border-gray-600" />
                    Changes
                  </label>
                  <label className="flex items-center gap-2 text-sm text-gray-300">
                    <input type="checkbox" checked={newPath.alert_on_delete} onChange={(e) => setNewPath({ ...newPath, alert_on_delete: e.target.checked })}
                      className="rounded bg-gray-700 border-gray-600" />
                    Deletions
                  </label>
                  <label className="flex items-center gap-2 text-sm text-gray-300">
                    <input type="checkbox" checked={newPath.alert_on_create} onChange={(e) => setNewPath({ ...newPath, alert_on_create: e.target.checked })}
                      className="rounded bg-gray-700 border-gray-600" />
                    New Files
                  </label>
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Ignore Patterns (comma-separated globs)</label>
                <input type="text" value={newPath.ignore_patterns} onChange={(e) => setNewPath({ ...newPath, ignore_patterns: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500"
                  placeholder="*.log, *.tmp, .git" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Description</label>
                <input type="text" value={newPath.description} onChange={(e) => setNewPath({ ...newPath, description: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500"
                  placeholder="Nginx main config" />
              </div>
            </div>
            <div className="p-6 border-t border-gray-700 flex justify-end gap-3">
              <button onClick={() => setShowCreate(false)} className="px-4 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg transition-colors">Cancel</button>
              <button onClick={handleCreatePath} disabled={!newPath.path}
                className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors disabled:opacity-50">Add Path</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
