'use client';

import { useState, useEffect } from 'react';
import { Shield, Plus, Trash2, RefreshCw, Eye, EyeOff, AlertTriangle, Lock, FolderOpen, ToggleLeft, ToggleRight } from 'lucide-react';

interface ProtectionRule {
  id: string;
  name: string;
  path: string;
  recursive: boolean;
  file_pattern: string;
  watch_create: boolean;
  watch_modify: boolean;
  watch_delete: boolean;
  watch_permissions: boolean;
  is_active: boolean;
  created_at: string;
}

interface ChangeEvent {
  id: string;
  rule_id: string;
  file_path: string;
  event_type: string;
  old_hash: string;
  new_hash: string;
  details: string;
  severity: string;
  is_read: boolean;
  created_at: string;
}

interface QuarantineItem {
  id: string;
  original_path: string;
  quarantine_path: string;
  sha256_hash: string;
  file_size: number;
  reason: string;
  restored_at: string | null;
  created_at: string;
}

interface FileProtectionStats {
  total_rules: number;
  active_rules: number;
  total_files: number;
  changes_today: number;
  quarantined_files: number;
  unread_alerts: number;
}

const CARD_CLASS = 'rounded-lg border border-gray-200 bg-white shadow-sm';
const CARD_HEADER_CLASS =
  'flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-5 py-4';
const CARD_TITLE_CLASS = 'text-sm font-semibold text-gray-900';
const TH_CLASS = 'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
const BTN_PRIMARY =
  'inline-flex items-center gap-2 rounded-md bg-blue-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 disabled:opacity-50';
const BTN_SECONDARY =
  'inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 disabled:opacity-50';
const INPUT_CLASS =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500';
const BADGE_BASE = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium';

function formatDateTime(value: string): string {
  if (!value) return '—';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return '—';
  return parsed.toLocaleString();
}

function formatDate(value: string): string {
  if (!value) return '—';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return '—';
  return parsed.toLocaleDateString();
}

function formatSize(bytes: number | undefined | null): string {
  if (typeof bytes !== 'number' || !Number.isFinite(bytes)) return '0.0 KB';
  return `${(bytes / 1024).toFixed(1)} KB`;
}

export default function FileProtectionPage() {
  const [stats, setStats] = useState<FileProtectionStats | null>(null);
  const [rules, setRules] = useState<ProtectionRule[]>([]);
  const [events, setEvents] = useState<ChangeEvent[]>([]);
  const [quarantine, setQuarantine] = useState<QuarantineItem[]>([]);
  const [activeTab, setActiveTab] = useState('rules');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showRuleModal, setShowRuleModal] = useState(false);
  const [newRule, setNewRule] = useState({
    name: '', path: '', recursive: true, file_pattern: '*',
    watch_create: true, watch_modify: true, watch_delete: true, watch_permissions: true,
  });

  const getToken = () => {
    if (typeof window === 'undefined') return '';
    try {
      return localStorage.getItem('access_token') || '';
    } catch {
      return '';
    }
  };

  const fetchAll = async () => {
    setLoading(true);
    setError(null);
    const token = getToken();
    const headers = { Authorization: `Bearer ${token}` };
    try {
      const [statsRes, rulesRes, eventsRes, quarantineRes] = await Promise.all([
        fetch('/api/v1/file-protection/stats', { headers }),
        fetch('/api/v1/file-protection/rules', { headers }),
        fetch('/api/v1/file-protection/events?limit=50', { headers }),
        fetch('/api/v1/file-protection/quarantine', { headers }),
      ]);
      if (statsRes.ok) { const d = await statsRes.json(); setStats(d?.stats ?? null); } else { setStats(null); }
      if (rulesRes.ok) { const d = await rulesRes.json(); setRules(Array.isArray(d?.rules) ? d.rules : []); } else { setRules([]); }
      if (eventsRes.ok) { const d = await eventsRes.json(); setEvents(Array.isArray(d?.events) ? d.events : []); } else { setEvents([]); }
      if (quarantineRes.ok) { const d = await quarantineRes.json(); setQuarantine(Array.isArray(d?.quarantine) ? d.quarantine : []); } else { setQuarantine([]); }
    } catch (e) {
      console.error(e);
      setStats(null);
      setRules([]);
      setEvents([]);
      setQuarantine([]);
      setError('Failed to load file protection data. Please try again.');
    }
    setLoading(false);
  };

  useEffect(() => { fetchAll(); }, []);

  const createRule = async () => {
    try {
      const token = getToken();
      await fetch('/api/v1/file-protection/rules', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify(newRule),
      });
      setShowRuleModal(false);
      setNewRule({ name: '', path: '', recursive: true, file_pattern: '*', watch_create: true, watch_modify: true, watch_delete: true, watch_permissions: true });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Failed to create protection rule.');
    }
  };

  const deleteRule = async (id: string) => {
    if (!confirm('Delete this protection rule?')) return;
    try {
      const token = getToken();
      await fetch(`/api/v1/file-protection/rules/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Failed to delete protection rule.');
    }
  };

  const toggleRule = async (id: string) => {
    try {
      const token = getToken();
      await fetch(`/api/v1/file-protection/rules/${id}/toggle`, { method: 'POST', headers: { Authorization: `Bearer ${token}` } });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Failed to toggle protection rule.');
    }
  };

  const markAllRead = async () => {
    try {
      const token = getToken();
      await fetch('/api/v1/file-protection/events/read-all', { method: 'PUT', headers: { Authorization: `Bearer ${token}` } });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Failed to mark events as read.');
    }
  };

  const restoreQuarantine = async (id: string) => {
    try {
      const token = getToken();
      await fetch(`/api/v1/file-protection/quarantine/${id}/restore`, { method: 'POST', headers: { Authorization: `Bearer ${token}` } });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Failed to restore quarantined file.');
    }
  };

  const deleteQuarantine = async (id: string) => {
    if (!confirm('Permanently delete this quarantined file?')) return;
    try {
      const token = getToken();
      await fetch(`/api/v1/file-protection/quarantine/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Failed to delete quarantined file.');
    }
  };

  const severityColor = (s: string) => {
    switch (s) {
      case 'critical': return 'bg-red-50 text-red-700';
      case 'high': return 'bg-orange-50 text-orange-700';
      case 'medium': return 'bg-amber-50 text-amber-700';
      default: return 'bg-blue-50 text-blue-700';
    }
  };

  const eventTypeColor = (type: string) => {
    switch (type) {
      case 'created': return 'bg-emerald-50 text-emerald-700';
      case 'modified': return 'bg-amber-50 text-amber-700';
      case 'deleted': return 'bg-red-50 text-red-700';
      default: return 'bg-sky-50 text-sky-700';
    }
  };

  const tabs = [
    { id: 'rules', label: 'Protection Rules', icon: Shield },
    { id: 'events', label: 'Change Events', icon: AlertTriangle },
    { id: 'quarantine', label: 'Quarantine', icon: Lock },
  ];

  const unreadAlerts = stats?.unread_alerts ?? 0;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold text-gray-900">
            <Shield className="h-5 w-5 text-gray-500" aria-hidden="true" /> File Protection
          </h1>
          <p className="mt-1 text-sm text-gray-600">
            Monitor file integrity and detect unauthorized changes
          </p>
        </div>
        <button type="button" onClick={fetchAll} className={BTN_SECONDARY}>
          <RefreshCw className="h-4 w-4" aria-hidden="true" /> Refresh
        </button>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
          {error}
        </div>
      )}

      {loading ? (
        <div className="flex h-48 items-center justify-center rounded-lg border border-gray-200 bg-white">
          <RefreshCw className="h-6 w-6 animate-spin text-gray-400" aria-hidden="true" />
          <span className="ml-2 text-sm text-gray-600">Loading file protection data...</span>
        </div>
      ) : (
        <>
          {/* Stats Cards */}
          {stats && (
            <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6">
              {[
                { label: 'Total Rules', value: stats.total_rules ?? 0 },
                { label: 'Active Rules', value: stats.active_rules ?? 0 },
                { label: 'Monitored Files', value: stats.total_files ?? 0 },
                { label: 'Changes Today', value: stats.changes_today ?? 0 },
                { label: 'Quarantined', value: stats.quarantined_files ?? 0 },
                { label: 'Unread Alerts', value: stats.unread_alerts ?? 0 },
              ].map((s) => (
                <div key={s.label} className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
                  <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{s.label}</p>
                  <p className="mt-1 text-2xl font-semibold text-gray-900">{s.value}</p>
                </div>
              ))}
            </div>
          )}

          {/* Tabs */}
          <div className="border-b border-gray-200">
            <nav className="flex gap-6" aria-label="File protection sections">
              {tabs.map((tab) => (
                <button
                  key={tab.id}
                  type="button"
                  onClick={() => setActiveTab(tab.id)}
                  aria-current={activeTab === tab.id ? 'page' : undefined}
                  className={`-mb-px flex items-center gap-2 border-b-2 px-1 py-3 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                    activeTab === tab.id
                      ? 'border-blue-600 text-blue-700'
                      : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-900'
                  }`}
                >
                  <tab.icon className="h-4 w-4" aria-hidden="true" />
                  {tab.label}
                  {tab.id === 'events' && unreadAlerts > 0 && (
                    <span className={`${BADGE_BASE} ml-1 bg-red-50 text-red-700`}>{unreadAlerts}</span>
                  )}
                </button>
              ))}
            </nav>
          </div>

          {/* Rules Tab */}
          {activeTab === 'rules' && (
            <div className={CARD_CLASS}>
              <div className={CARD_HEADER_CLASS}>
                <h3 className={CARD_TITLE_CLASS}>Protection Rules</h3>
                <button type="button" onClick={() => setShowRuleModal(true)} className={BTN_PRIMARY}>
                  <Plus className="h-4 w-4" aria-hidden="true" /> Add Rule
                </button>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr className="[&_th]:border-b [&_th]:border-gray-200">
                      <th className={TH_CLASS}>Name</th>
                      <th className={TH_CLASS}>Path</th>
                      <th className={TH_CLASS}>Pattern</th>
                      <th className={TH_CLASS}>Watching</th>
                      <th className={TH_CLASS}>Status</th>
                      <th className={`${TH_CLASS} text-right`}>Actions</th>
                    </tr>
                  </thead>
                  <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                    {rules.length === 0 ? (
                      <tr>
                        <td colSpan={6} className="px-4 py-12 text-center">
                          <FolderOpen className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                          <p className="mt-3 text-sm font-semibold text-gray-900">No protection rules configured</p>
                          <p className="mt-1 text-sm text-gray-600">Add a rule to start monitoring a directory.</p>
                        </td>
                      </tr>
                    ) : rules.map((r) => (
                      <tr key={r.id} className="hover:bg-gray-50">
                        <td className="px-4 py-3 text-sm font-medium text-gray-900">{r.name || '—'}</td>
                        <td className="px-4 py-3 font-mono text-sm text-gray-600">{r.path || '—'}</td>
                        <td className="px-4 py-3 font-mono text-sm text-gray-600">{r.file_pattern || '—'}</td>
                        <td className="px-4 py-3">
                          <div className="flex gap-1">
                            {r.watch_create && <span className={`${BADGE_BASE} bg-emerald-50 text-emerald-700`} title="Create">C</span>}
                            {r.watch_modify && <span className={`${BADGE_BASE} bg-blue-50 text-blue-700`} title="Modify">M</span>}
                            {r.watch_delete && <span className={`${BADGE_BASE} bg-red-50 text-red-700`} title="Delete">D</span>}
                            {r.watch_permissions && <span className={`${BADGE_BASE} bg-sky-50 text-sky-700`} title="Permissions">P</span>}
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <button
                            type="button"
                            onClick={() => toggleRule(r.id)}
                            aria-label={r.is_active ? `Disable rule ${r.name}` : `Enable rule ${r.name}`}
                            className="inline-flex items-center gap-1.5 rounded-md px-1 py-0.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                          >
                            {r.is_active
                              ? <ToggleRight className="h-5 w-5 text-emerald-600" aria-hidden="true" />
                              : <ToggleLeft className="h-5 w-5 text-gray-400" aria-hidden="true" />}
                            <span className={`text-xs font-medium ${r.is_active ? 'text-emerald-700' : 'text-gray-500'}`}>
                              {r.is_active ? 'Active' : 'Disabled'}
                            </span>
                          </button>
                        </td>
                        <td className="px-4 py-3 text-right">
                          <button
                            type="button"
                            onClick={() => deleteRule(r.id)}
                            aria-label={`Delete rule ${r.name}`}
                            title="Delete rule"
                            className="inline-flex items-center rounded-md p-1.5 text-red-600 hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
                          >
                            <Trash2 className="h-4 w-4" aria-hidden="true" />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* Events Tab */}
          {activeTab === 'events' && (
            <div className={CARD_CLASS}>
              <div className={CARD_HEADER_CLASS}>
                <h3 className={CARD_TITLE_CLASS}>Change Events</h3>
                <button type="button" onClick={markAllRead} className={BTN_SECONDARY}>
                  <Eye className="h-4 w-4" aria-hidden="true" /> Mark All Read
                </button>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr className="[&_th]:border-b [&_th]:border-gray-200">
                      <th className={TH_CLASS}>File</th>
                      <th className={TH_CLASS}>Event</th>
                      <th className={TH_CLASS}>Severity</th>
                      <th className={TH_CLASS}>Details</th>
                      <th className={TH_CLASS}>Time</th>
                    </tr>
                  </thead>
                  <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                    {events.length === 0 ? (
                      <tr>
                        <td colSpan={5} className="px-4 py-12 text-center">
                          <EyeOff className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                          <p className="mt-3 text-sm font-semibold text-gray-900">No change events detected</p>
                          <p className="mt-1 text-sm text-gray-600">Monitored files are unchanged.</p>
                        </td>
                      </tr>
                    ) : events.map((e) => (
                      <tr key={e.id} className={`hover:bg-gray-50 ${!e.is_read ? 'bg-blue-50' : ''}`}>
                        <td className="max-w-[250px] truncate px-4 py-3 font-mono text-sm text-gray-900">{e.file_path || '—'}</td>
                        <td className="px-4 py-3">
                          <span className={`${BADGE_BASE} ${eventTypeColor(e.event_type)}`}>
                            {e.event_type || 'unknown'}
                          </span>
                        </td>
                        <td className="px-4 py-3">
                          <span className={`${BADGE_BASE} ${severityColor(e.severity)}`}>{e.severity || 'low'}</span>
                        </td>
                        <td className="max-w-[200px] truncate px-4 py-3 text-sm text-gray-600">{e.details || '—'}</td>
                        <td className="px-4 py-3 text-sm text-gray-600" suppressHydrationWarning>{formatDateTime(e.created_at)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* Quarantine Tab */}
          {activeTab === 'quarantine' && (
            <div className={CARD_CLASS}>
              <div className={CARD_HEADER_CLASS}>
                <h3 className={CARD_TITLE_CLASS}>Quarantined Files</h3>
                <span className="text-xs text-gray-500">{quarantine.length} items</span>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr className="[&_th]:border-b [&_th]:border-gray-200">
                      <th className={TH_CLASS}>Original Path</th>
                      <th className={TH_CLASS}>Reason</th>
                      <th className={TH_CLASS}>Size</th>
                      <th className={TH_CLASS}>Status</th>
                      <th className={TH_CLASS}>Date</th>
                      <th className={`${TH_CLASS} text-right`}>Actions</th>
                    </tr>
                  </thead>
                  <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                    {quarantine.length === 0 ? (
                      <tr>
                        <td colSpan={6} className="px-4 py-12 text-center">
                          <Lock className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                          <p className="mt-3 text-sm font-semibold text-gray-900">No quarantined files</p>
                          <p className="mt-1 text-sm text-gray-600">Suspicious files moved to quarantine will be listed here.</p>
                        </td>
                      </tr>
                    ) : quarantine.map((q) => (
                      <tr key={q.id} className="hover:bg-gray-50">
                        <td className="max-w-[250px] truncate px-4 py-3 font-mono text-sm text-gray-900">{q.original_path || '—'}</td>
                        <td className="max-w-[200px] truncate px-4 py-3 text-sm text-gray-600">{q.reason || '—'}</td>
                        <td className="px-4 py-3 text-sm text-gray-600">{formatSize(q.file_size)}</td>
                        <td className="px-4 py-3">
                          <span className={`${BADGE_BASE} ${q.restored_at ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'}`}>
                            {q.restored_at ? 'Restored' : 'Quarantined'}
                          </span>
                        </td>
                        <td className="px-4 py-3 text-sm text-gray-600" suppressHydrationWarning>{formatDate(q.created_at)}</td>
                        <td className="px-4 py-3 text-right">
                          <div className="flex items-center justify-end gap-2">
                            {!q.restored_at && (
                              <button
                                type="button"
                                onClick={() => restoreQuarantine(q.id)}
                                className="rounded-md px-2 py-1 text-sm font-medium text-blue-700 hover:bg-blue-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                              >
                                Restore
                              </button>
                            )}
                            <button
                              type="button"
                              onClick={() => deleteQuarantine(q.id)}
                              aria-label="Delete quarantined file"
                              title="Delete permanently"
                              className="inline-flex items-center rounded-md p-1.5 text-red-600 hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
                            >
                              <Trash2 className="h-4 w-4" aria-hidden="true" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </>
      )}

      {/* Create Rule Modal */}
      {showRuleModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/50 p-4" role="dialog" aria-modal="true" aria-labelledby="fp-rule-modal-title">
          <div className="w-full max-w-lg rounded-lg border border-gray-200 bg-white shadow-lg">
            <div className="border-b border-gray-200 px-5 py-4">
              <h3 id="fp-rule-modal-title" className="text-sm font-semibold text-gray-900">Add Protection Rule</h3>
            </div>
            <div className="space-y-4 px-5 py-4">
              <div>
                <label htmlFor="fp-rule-name" className="mb-1 block text-sm font-medium text-gray-700">Rule name</label>
                <input
                  id="fp-rule-name"
                  value={newRule.name}
                  onChange={(e) => setNewRule({ ...newRule, name: e.target.value })}
                  placeholder="Rule name"
                  className={INPUT_CLASS}
                />
              </div>
              <div>
                <label htmlFor="fp-rule-path" className="mb-1 block text-sm font-medium text-gray-700">Path</label>
                <input
                  id="fp-rule-path"
                  value={newRule.path}
                  onChange={(e) => setNewRule({ ...newRule, path: e.target.value })}
                  placeholder="/etc/nginx"
                  className={`${INPUT_CLASS} font-mono`}
                />
              </div>
              <div>
                <label htmlFor="fp-rule-pattern" className="mb-1 block text-sm font-medium text-gray-700">File pattern</label>
                <input
                  id="fp-rule-pattern"
                  value={newRule.file_pattern}
                  onChange={(e) => setNewRule({ ...newRule, file_pattern: e.target.value })}
                  placeholder="*.conf"
                  className={`${INPUT_CLASS} font-mono`}
                />
              </div>
              <label className="flex items-center gap-2 text-sm text-gray-700">
                <input
                  type="checkbox"
                  checked={newRule.recursive}
                  onChange={(e) => setNewRule({ ...newRule, recursive: e.target.checked })}
                  className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                /> Recursive
              </label>
              <div className="grid grid-cols-2 gap-2">
                {(['watch_create', 'watch_modify', 'watch_delete', 'watch_permissions'] as const).map((key) => (
                  <label key={key} className="flex items-center gap-2 text-sm text-gray-700">
                    <input
                      type="checkbox"
                      checked={newRule[key]}
                      onChange={(e) => setNewRule({ ...newRule, [key]: e.target.checked })}
                      className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                    />
                    {key.replace('watch_', '').charAt(0).toUpperCase() + key.replace('watch_', '').slice(1)}
                  </label>
                ))}
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              <button type="button" onClick={() => setShowRuleModal(false)} className={BTN_SECONDARY}>Cancel</button>
              <button type="button" onClick={createRule} className={BTN_PRIMARY}>Create</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
