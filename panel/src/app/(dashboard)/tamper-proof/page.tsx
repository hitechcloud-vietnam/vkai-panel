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

const CARD_CLASS = 'rounded-lg border border-gray-200 bg-white shadow-sm';
const CARD_HEADER_CLASS =
  'flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-5 py-4';
const CARD_TITLE_CLASS = 'text-sm font-semibold text-gray-900';
const BTN_PRIMARY =
  'inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 disabled:opacity-50';
const BTN_SECONDARY =
  'inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 disabled:opacity-50';
const INPUT_CLASS =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500';
const LABEL_CLASS = 'mb-1 block text-sm font-medium text-gray-700';
const BADGE_BASE = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium';
const ICON_BTN =
  'inline-flex items-center rounded-md p-1.5 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 disabled:opacity-50';

function formatDateTime(value: string | null | undefined): string {
  if (!value) return '—';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return '—';
  return parsed.toLocaleString();
}

function shortHash(value: string | undefined | null, length = 16): string {
  if (!value) return '';
  return value.length > length ? `${value.substring(0, length)}...` : value;
}

export default function TamperProofPage() {
  const [stats, setStats] = useState<TamperStats | null>(null);
  const [paths, setPaths] = useState<ProtectedPath[]>([]);
  const [alerts, setAlerts] = useState<TamperAlert[]>([]);
  const [scanResults, setScanResults] = useState<ScanResult[]>([]);
  const [activeTab, setActiveTab] = useState<'paths' | 'alerts' | 'scans' | 'audit'>('paths');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
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
      try {
        const authStorage = localStorage.getItem('auth-storage');
        if (authStorage) {
          const parsed = JSON.parse(authStorage);
          return parsed?.state?.token || '';
        }
      } catch { return ''; }
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
    setError(null);
    try {
      const [statsRes, pathsRes, alertsRes, scansRes, auditRes] = await Promise.all([
        apiCall('/v1/tamper-proof/stats').catch(() => ({ stats: null })),
        apiCall('/v1/tamper-proof/paths').catch(() => ({ paths: [] })),
        apiCall('/v1/tamper-proof/alerts').catch(() => ({ alerts: [] })),
        apiCall('/v1/tamper-proof/scan-results?limit=30').catch(() => ({ results: [] })),
        apiCall('/v1/tamper-proof/audit-logs?limit=50').catch(() => ({ logs: [] })),
      ]);
      setStats(statsRes?.stats ?? null);
      setPaths(Array.isArray(pathsRes?.paths) ? pathsRes.paths : []);
      setAlerts(Array.isArray(alertsRes?.alerts) ? alertsRes.alerts : []);
      setScanResults(Array.isArray(scansRes?.results) ? scansRes.results : []);
      setAuditLogs(Array.isArray(auditRes?.logs) ? auditRes.logs : []);
    } catch (err) {
      console.error('Failed to fetch data:', err);
      setStats(null);
      setPaths([]);
      setAlerts([]);
      setScanResults([]);
      setAuditLogs([]);
      setError('Failed to load tamper-proof data. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchAll(); }, []);

  const handleCreatePath = async () => {
    try {
      setError(null);
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
      setError('Failed to create protected path.');
    }
  };

  const handleScan = async (id: string) => {
    setScanning(id);
    try {
      setError(null);
      await apiCall(`/v1/tamper-proof/paths/${id}/scan`, { method: 'POST' });
      await fetchAll();
    } catch (err) {
      console.error('Scan failed:', err);
      setError('Scan failed. Please try again.');
    } finally {
      setScanning(null);
    }
  };

  const handleScanAll = async () => {
    setScanning('all');
    try {
      setError(null);
      await apiCall('/v1/tamper-proof/scan-all', { method: 'POST' });
      await fetchAll();
    } catch (err) {
      console.error('Scan all failed:', err);
      setError('Scan all failed. Please try again.');
    } finally {
      setScanning(null);
    }
  };

  const handleDeletePath = async (id: string) => {
    if (!confirm('Delete this protected path and all its baselines?')) return;
    try {
      setError(null);
      await apiCall(`/v1/tamper-proof/paths/${id}`, { method: 'DELETE' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to delete:', err);
      setError('Failed to delete protected path.');
    }
  };

  const handleResolveAlert = async (id: string) => {
    try {
      setError(null);
      await apiCall(`/v1/tamper-proof/alerts/${id}/resolve`, {
        method: 'POST',
        body: JSON.stringify({ notes: 'Resolved by admin' }),
      });
      await fetchAll();
    } catch (err) {
      console.error('Failed to resolve:', err);
      setError('Failed to resolve alert.');
    }
  };

  const handleRefreshBaseline = async (id: string) => {
    if (!confirm('This will re-scan and update all checksums. Continue?')) return;
    try {
      setError(null);
      await apiCall(`/v1/tamper-proof/paths/${id}/baselines/refresh`, { method: 'POST' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to refresh:', err);
      setError('Failed to refresh baseline.');
    }
  };

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case 'critical': return 'text-red-700 bg-red-50';
      case 'high': return 'text-orange-700 bg-orange-50';
      case 'medium': return 'text-amber-700 bg-amber-50';
      case 'low': return 'text-brand-700 bg-brand-50';
      default: return 'text-gray-700 bg-gray-100';
    }
  };

  const getAlertTypeIcon = (type: string) => {
    switch (type) {
      case 'modified': return <AlertTriangle size={16} className="text-amber-600" aria-hidden="true" />;
      case 'deleted': return <XCircle size={16} className="text-red-600" aria-hidden="true" />;
      case 'created': return <Plus size={16} className="text-brand-600" aria-hidden="true" />;
      default: return <AlertTriangle size={16} className="text-gray-500" aria-hidden="true" />;
    }
  };

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <RefreshCw className="h-6 w-6 animate-spin text-gray-400" aria-hidden="true" />
        <span className="ml-2 text-sm text-gray-600">Loading tamper-proof data...</span>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Tamper Proof for Enterprise</h1>
          <p className="mt-1 text-sm text-gray-600">
            File integrity monitoring, intrusion detection &amp; audit trail
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button type="button" onClick={fetchAll} className={BTN_SECONDARY}>
            <RefreshCw size={16} aria-hidden="true" /> Refresh
          </button>
          <button type="button" onClick={handleScanAll} disabled={scanning !== null} className={BTN_SECONDARY}>
            <Scan size={16} aria-hidden="true" /> {scanning === 'all' ? 'Scanning...' : 'Scan All'}
          </button>
          <button type="button" onClick={() => setShowCreate(true)} className={BTN_PRIMARY}>
            <Plus size={16} aria-hidden="true" /> Add Path
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
          {error}
        </div>
      )}

      {/* Stats */}
      {stats && (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-5">
          {[
            { label: 'Protected Paths', value: stats.protected_paths ?? 0, icon: <Shield size={18} aria-hidden="true" /> },
            { label: 'Monitored Files', value: stats.total_files ?? 0, icon: <FileCheck size={18} aria-hidden="true" /> },
            { label: 'Active Alerts', value: stats.active_alerts ?? 0, icon: <AlertTriangle size={18} aria-hidden="true" /> },
            { label: 'Clean Scans', value: stats.clean_scans ?? 0, icon: <CheckCircle size={18} aria-hidden="true" /> },
            { label: 'Total Scans', value: stats.total_scans ?? 0, icon: <History size={18} aria-hidden="true" /> },
          ].map((stat, i) => (
            <div key={i} className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
              <div className="flex items-start justify-between">
                <div>
                  <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{stat.label}</p>
                  <p className="mt-1 text-2xl font-semibold text-gray-900">{stat.value}</p>
                </div>
                <div className="text-gray-400">{stat.icon}</div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <nav className="flex gap-6" aria-label="Tamper proof sections">
          {(['paths', 'alerts', 'scans', 'audit'] as const).map((tab) => (
            <button
              key={tab}
              type="button"
              onClick={() => setActiveTab(tab)}
              aria-current={activeTab === tab ? 'page' : undefined}
              className={`-mb-px border-b-2 px-1 py-3 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 ${
                activeTab === tab
                  ? 'border-brand-600 text-brand-700'
                  : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-900'
              }`}
            >
              {tab === 'paths'
                ? 'Protected Paths'
                : tab === 'alerts'
                ? `Alerts${stats?.active_alerts ? ` (${stats.active_alerts})` : ''}`
                : tab === 'scans'
                ? 'Scan Results'
                : 'Audit Log'}
            </button>
          ))}
        </nav>
      </div>

      {/* Protected Paths Tab */}
      {activeTab === 'paths' && (
        <div className={CARD_CLASS}>
          <div className={CARD_HEADER_CLASS}>
            <h2 className={CARD_TITLE_CLASS}>Protected Paths</h2>
            <span className="text-xs text-gray-500">{paths.length} paths</span>
          </div>
          <div className="divide-y divide-gray-100">
            {paths.length === 0 ? (
              <div className="px-5 py-12 text-center">
                <Lock className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                <p className="mt-3 text-sm font-semibold text-gray-900">No protected paths configured</p>
                <p className="mt-1 text-sm text-gray-600">Add a file or directory to monitor.</p>
              </div>
            ) : (
              paths.map((p) => (
                <div key={p.id} className="px-5 py-4 hover:bg-gray-50">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="flex items-start gap-3">
                      <div className={`rounded-md p-2 ${p.is_enabled ? 'bg-emerald-50' : 'bg-gray-100'}`}>
                        {p.is_enabled
                          ? <Lock size={16} className="text-emerald-600" aria-hidden="true" />
                          : <Unlock size={16} className="text-gray-500" aria-hidden="true" />}
                      </div>
                      <div>
                        <div className="flex flex-wrap items-center gap-2">
                          <p className="font-mono text-sm text-gray-900">{p.path || '—'}</p>
                          <span className={`${BADGE_BASE} bg-gray-100 text-gray-700`}>{p.path_type || 'file'}</span>
                          <span className={`${BADGE_BASE} bg-brand-50 text-brand-700`}>{p.algorithm || 'sha256'}</span>
                          {p.recursive && <span className={`${BADGE_BASE} bg-sky-50 text-sky-700`}>recursive</span>}
                        </div>
                        <div className="mt-1 flex flex-wrap items-center gap-4 text-xs text-gray-500">
                          <span>Files: {p.file_count ?? 0}</span>
                          <span>
                            Alerts: {p.alert_on_change ? 'change' : ''} {p.alert_on_delete ? 'delete' : ''} {p.alert_on_create ? 'create' : ''}
                          </span>
                          {p.last_scan_at && (
                            <span suppressHydrationWarning>Last scan: {formatDateTime(p.last_scan_at)}</span>
                          )}
                        </div>
                        {p.description && <p className="mt-1 text-xs text-gray-600">{p.description}</p>}
                      </div>
                    </div>
                    <div className="flex items-center gap-1">
                      <button
                        type="button"
                        onClick={() => handleScan(p.id)}
                        disabled={scanning === p.id}
                        className={ICON_BTN}
                        aria-label={`Scan ${p.path} now`}
                        title="Scan Now"
                      >
                        <Scan size={16} aria-hidden="true" />
                      </button>
                      <button
                        type="button"
                        onClick={() => handleRefreshBaseline(p.id)}
                        className={ICON_BTN}
                        aria-label={`Refresh baseline for ${p.path}`}
                        title="Refresh Baseline"
                      >
                        <RefreshCw size={16} aria-hidden="true" />
                      </button>
                      <button
                        type="button"
                        onClick={() => handleDeletePath(p.id)}
                        className="inline-flex items-center rounded-md p-1.5 text-red-600 hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
                        aria-label={`Delete protected path ${p.path}`}
                        title="Delete"
                      >
                        <Trash2 size={16} aria-hidden="true" />
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
        <div className={CARD_CLASS}>
          <div className={CARD_HEADER_CLASS}>
            <h2 className={CARD_TITLE_CLASS}>Integrity Alerts</h2>
            <span className="text-xs text-gray-500">{alerts.length} alerts</span>
          </div>
          <div className="divide-y divide-gray-100">
            {alerts.length === 0 ? (
              <div className="px-5 py-12 text-center">
                <CheckCircle className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                <p className="mt-3 text-sm font-semibold text-gray-900">No alerts</p>
                <p className="mt-1 text-sm text-gray-600">All monitored files are clean.</p>
              </div>
            ) : (
              alerts.map((alert) => (
                <div key={alert.id} className={`px-5 py-4 ${alert.is_resolved ? 'bg-gray-50' : ''}`}>
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="flex items-start gap-3">
                      <div className="mt-0.5">{getAlertTypeIcon(alert.alert_type)}</div>
                      <div>
                        <div className="flex flex-wrap items-center gap-2">
                          <p className="font-mono text-sm text-gray-900">{alert.file_path || '—'}</p>
                          <span className={`${BADGE_BASE} ${getSeverityColor(alert.severity)}`}>
                            {alert.severity || 'unknown'}
                          </span>
                          <span className={`${BADGE_BASE} bg-gray-100 text-gray-700`}>{alert.alert_type || 'event'}</span>
                          {alert.is_resolved && (
                            <span className={`${BADGE_BASE} bg-emerald-50 text-emerald-700`}>resolved</span>
                          )}
                        </div>
                        <div className="mt-1 flex flex-wrap items-center gap-4 text-xs text-gray-500">
                          {alert.old_checksum && <span className="font-mono">Old: {shortHash(alert.old_checksum)}</span>}
                          {alert.new_checksum && <span className="font-mono">New: {shortHash(alert.new_checksum)}</span>}
                          <span suppressHydrationWarning>{formatDateTime(alert.created_at)}</span>
                        </div>
                      </div>
                    </div>
                    {!alert.is_resolved && (
                      <button
                        type="button"
                        onClick={() => handleResolveAlert(alert.id)}
                        className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
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
        <div className={CARD_CLASS}>
          <div className={CARD_HEADER_CLASS}>
            <h2 className={CARD_TITLE_CLASS}>Scan History</h2>
            <span className="text-xs text-gray-500">{scanResults.length} scans</span>
          </div>
          <div className="divide-y divide-gray-100">
            {scanResults.length === 0 ? (
              <div className="px-5 py-12 text-center">
                <History className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                <p className="mt-3 text-sm font-semibold text-gray-900">No scan results yet</p>
                <p className="mt-1 text-sm text-gray-600">Run a scan to build the integrity history.</p>
              </div>
            ) : (
              scanResults.map((scan) => (
                <div key={scan.id} className="px-5 py-4">
                  <div className="flex items-start gap-3">
                    <div className="mt-0.5">
                      {scan.status === 'clean'
                        ? <CheckCircle size={16} className="text-emerald-600" aria-hidden="true" />
                        : scan.status === 'violations_found'
                        ? <AlertTriangle size={16} className="text-red-600" aria-hidden="true" />
                        : <XCircle size={16} className="text-gray-400" aria-hidden="true" />}
                    </div>
                    <div className="min-w-0">
                      <span className={`text-sm font-medium ${scan.status === 'clean' ? 'text-emerald-700' : 'text-red-700'}`}>
                        {scan.status === 'clean'
                          ? 'Clean'
                          : scan.status === 'violations_found'
                          ? 'Violations Found'
                          : 'Error'}
                      </span>
                      <div className="mt-1 flex flex-wrap items-center gap-4 text-xs text-gray-500">
                        <span>Files: {scan.scanned_files ?? 0}/{scan.total_files ?? 0}</span>
                        {scan.violations > 0 && <span className="text-red-700">Violations: {scan.violations}</span>}
                        {scan.new_files > 0 && <span className="text-brand-700">New: {scan.new_files}</span>}
                        {scan.modified_files > 0 && <span className="text-amber-700">Modified: {scan.modified_files}</span>}
                        {scan.deleted_files > 0 && <span className="text-red-700">Deleted: {scan.deleted_files}</span>}
                        <span>Duration: {scan.duration ?? 0}ms</span>
                        <span suppressHydrationWarning>{formatDateTime(scan.created_at)}</span>
                      </div>
                      {scan.scan_log && (
                        <pre className="mt-2 overflow-x-auto rounded-md border border-gray-200 bg-gray-50 p-3 font-mono text-xs text-gray-700">
                          {scan.scan_log.substring(0, 200)}
                        </pre>
                      )}
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
        <div className={CARD_CLASS}>
          <div className={CARD_HEADER_CLASS}>
            <h2 className={CARD_TITLE_CLASS}>Audit Trail</h2>
            <span className="text-xs text-gray-500">{auditLogs.length} entries</span>
          </div>
          <div className="divide-y divide-gray-100">
            {auditLogs.length === 0 ? (
              <div className="px-5 py-12 text-center">
                <Eye className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                <p className="mt-3 text-sm font-semibold text-gray-900">No audit entries yet</p>
                <p className="mt-1 text-sm text-gray-600">Administrative actions will be recorded here.</p>
              </div>
            ) : (
              auditLogs.map((log) => (
                <div key={log?.id} className="flex flex-wrap items-start justify-between gap-3 px-5 py-4">
                  <div className="flex items-start gap-3">
                    <History size={16} className="mt-0.5 text-gray-400" aria-hidden="true" />
                    <div>
                      <p className="text-sm text-gray-900">
                        {log?.action || '—'}: <span className="text-gray-600">{log?.target || '—'}</span>
                      </p>
                      {log?.details && <p className="mt-1 text-xs text-gray-500">{log.details}</p>}
                    </div>
                  </div>
                  <div className="text-right">
                    <p className="text-xs font-medium text-gray-700">{log?.username || '—'}</p>
                    <p className="text-xs text-gray-500" suppressHydrationWarning>{formatDateTime(log?.created_at)}</p>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* Create Path Modal */}
      {showCreate && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/50 p-4"
          role="dialog"
          aria-modal="true"
          aria-labelledby="tp-create-title"
          onClick={() => setShowCreate(false)}
        >
          <div
            className="max-h-[80vh] w-full max-w-lg overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="border-b border-gray-200 px-5 py-4">
              <h3 id="tp-create-title" className="text-sm font-semibold text-gray-900">Add Protected Path</h3>
            </div>
            <div className="space-y-4 px-5 py-4">
              <div>
                <label htmlFor="tp-path" className={LABEL_CLASS}>Path</label>
                <input
                  id="tp-path"
                  type="text"
                  value={newPath.path}
                  onChange={(e) => setNewPath({ ...newPath, path: e.target.value })}
                  className={`${INPUT_CLASS} font-mono`}
                  placeholder="/etc/nginx/nginx.conf"
                />
              </div>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label htmlFor="tp-type" className={LABEL_CLASS}>Type</label>
                  <select
                    id="tp-type"
                    value={newPath.path_type}
                    onChange={(e) => setNewPath({ ...newPath, path_type: e.target.value })}
                    className={INPUT_CLASS}
                  >
                    <option value="file">File</option>
                    <option value="directory">Directory</option>
                  </select>
                </div>
                <div>
                  <label htmlFor="tp-algorithm" className={LABEL_CLASS}>Algorithm</label>
                  <select
                    id="tp-algorithm"
                    value={newPath.algorithm}
                    onChange={(e) => setNewPath({ ...newPath, algorithm: e.target.value })}
                    className={INPUT_CLASS}
                  >
                    <option value="sha256">SHA-256</option>
                    <option value="sha512">SHA-512</option>
                    <option value="md5">MD5</option>
                  </select>
                </div>
              </div>
              <div className="flex items-center gap-4">
                <label className="flex items-center gap-2 text-sm text-gray-700">
                  <input
                    type="checkbox"
                    checked={newPath.recursive}
                    onChange={(e) => setNewPath({ ...newPath, recursive: e.target.checked })}
                    className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                  />
                  Recursive (directories)
                </label>
              </div>
              <div className="space-y-2">
                <p className="text-sm font-medium text-gray-700">Alert On:</p>
                <div className="flex flex-wrap items-center gap-4">
                  <label className="flex items-center gap-2 text-sm text-gray-700">
                    <input
                      type="checkbox"
                      checked={newPath.alert_on_change}
                      onChange={(e) => setNewPath({ ...newPath, alert_on_change: e.target.checked })}
                      className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                    />
                    Changes
                  </label>
                  <label className="flex items-center gap-2 text-sm text-gray-700">
                    <input
                      type="checkbox"
                      checked={newPath.alert_on_delete}
                      onChange={(e) => setNewPath({ ...newPath, alert_on_delete: e.target.checked })}
                      className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                    />
                    Deletions
                  </label>
                  <label className="flex items-center gap-2 text-sm text-gray-700">
                    <input
                      type="checkbox"
                      checked={newPath.alert_on_create}
                      onChange={(e) => setNewPath({ ...newPath, alert_on_create: e.target.checked })}
                      className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                    />
                    New Files
                  </label>
                </div>
              </div>
              <div>
                <label htmlFor="tp-ignore" className={LABEL_CLASS}>Ignore Patterns (comma-separated globs)</label>
                <input
                  id="tp-ignore"
                  type="text"
                  value={newPath.ignore_patterns}
                  onChange={(e) => setNewPath({ ...newPath, ignore_patterns: e.target.value })}
                  className={INPUT_CLASS}
                  placeholder="*.log, *.tmp, .git"
                />
              </div>
              <div>
                <label htmlFor="tp-description" className={LABEL_CLASS}>Description</label>
                <input
                  id="tp-description"
                  type="text"
                  value={newPath.description}
                  onChange={(e) => setNewPath({ ...newPath, description: e.target.value })}
                  className={INPUT_CLASS}
                  placeholder="Nginx main config"
                />
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              <button type="button" onClick={() => setShowCreate(false)} className={BTN_SECONDARY}>Cancel</button>
              <button type="button" onClick={handleCreatePath} disabled={!newPath.path} className={BTN_PRIMARY}>
                Add Path
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
