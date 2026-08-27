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

export default function FileProtectionPage() {
  const [stats, setStats] = useState<FileProtectionStats | null>(null);
  const [rules, setRules] = useState<ProtectionRule[]>([]);
  const [events, setEvents] = useState<ChangeEvent[]>([]);
  const [quarantine, setQuarantine] = useState<QuarantineItem[]>([]);
  const [activeTab, setActiveTab] = useState('rules');
  const [loading, setLoading] = useState(true);
  const [showRuleModal, setShowRuleModal] = useState(false);
  const [newRule, setNewRule] = useState({
    name: '', path: '', recursive: true, file_pattern: '*',
    watch_create: true, watch_modify: true, watch_delete: true, watch_permissions: true,
  });

  const getToken = () => localStorage.getItem('access_token') || '';

  const fetchAll = async () => {
    setLoading(true);
    const token = getToken();
    const headers = { Authorization: `Bearer ${token}` };
    try {
      const [statsRes, rulesRes, eventsRes, quarantineRes] = await Promise.all([
        fetch('/api/v1/file-protection/stats', { headers }),
        fetch('/api/v1/file-protection/rules', { headers }),
        fetch('/api/v1/file-protection/events?limit=50', { headers }),
        fetch('/api/v1/file-protection/quarantine', { headers }),
      ]);
      if (statsRes.ok) { const d = await statsRes.json(); setStats(d.stats); }
      if (rulesRes.ok) { const d = await rulesRes.json(); setRules(d.rules || []); }
      if (eventsRes.ok) { const d = await eventsRes.json(); setEvents(d.events || []); }
      if (quarantineRes.ok) { const d = await quarantineRes.json(); setQuarantine(d.quarantine || []); }
    } catch (e) { console.error(e); }
    setLoading(false);
  };

  useEffect(() => { fetchAll(); }, []);

  const createRule = async () => {
    const token = getToken();
    await fetch('/api/v1/file-protection/rules', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify(newRule),
    });
    setShowRuleModal(false);
    setNewRule({ name: '', path: '', recursive: true, file_pattern: '*', watch_create: true, watch_modify: true, watch_delete: true, watch_permissions: true });
    fetchAll();
  };

  const deleteRule = async (id: string) => {
    if (!confirm('Delete this protection rule?')) return;
    const token = getToken();
    await fetch(`/api/v1/file-protection/rules/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
    fetchAll();
  };

  const toggleRule = async (id: string) => {
    const token = getToken();
    await fetch(`/api/v1/file-protection/rules/${id}/toggle`, { method: 'POST', headers: { Authorization: `Bearer ${token}` } });
    fetchAll();
  };

  const markAllRead = async () => {
    const token = getToken();
    await fetch('/api/v1/file-protection/events/read-all', { method: 'PUT', headers: { Authorization: `Bearer ${token}` } });
    fetchAll();
  };

  const restoreQuarantine = async (id: string) => {
    const token = getToken();
    await fetch(`/api/v1/file-protection/quarantine/${id}/restore`, { method: 'POST', headers: { Authorization: `Bearer ${token}` } });
    fetchAll();
  };

  const deleteQuarantine = async (id: string) => {
    if (!confirm('Permanently delete this quarantined file?')) return;
    const token = getToken();
    await fetch(`/api/v1/file-protection/quarantine/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
    fetchAll();
  };

  const severityColor = (s: string) => {
    switch (s) {
      case 'critical': return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400';
      case 'high': return 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400';
      case 'medium': return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
      default: return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400';
    }
  };

  const tabs = [
    { id: 'rules', label: 'Protection Rules', icon: Shield },
    { id: 'events', label: 'Change Events', icon: AlertTriangle },
    { id: 'quarantine', label: 'Quarantine', icon: Lock },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
            <Shield className="w-6 h-6" /> File Protection
          </h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">Monitor file integrity and detect unauthorized changes</p>
        </div>
        <button onClick={fetchAll} className="flex items-center gap-2 px-4 py-2 bg-gray-100 dark:bg-gray-700 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600">
          <RefreshCw className="w-4 h-4" /> Refresh
        </button>
      </div>

      {/* Stats Cards */}
      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
          {[
            { label: 'Total Rules', value: stats.total_rules, color: 'blue' },
            { label: 'Active Rules', value: stats.active_rules, color: 'green' },
            { label: 'Monitored Files', value: stats.total_files, color: 'purple' },
            { label: 'Changes Today', value: stats.changes_today, color: 'orange' },
            { label: 'Quarantined', value: stats.quarantined_files, color: 'red' },
            { label: 'Unread Alerts', value: stats.unread_alerts, color: 'yellow' },
          ].map((s) => (
            <div key={s.label} className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700">
              <p className="text-sm text-gray-500 dark:text-gray-400">{s.label}</p>
              <p className="text-2xl font-bold text-gray-900 dark:text-white">{s.value}</p>
            </div>
          ))}
        </div>
      )}

      {/* Tabs */}
      <div className="border-b border-gray-200 dark:border-gray-700">
        <nav className="flex gap-4">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 py-3 px-1 border-b-2 text-sm font-medium ${
                activeTab === tab.id
                  ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                  : 'border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'
              }`}
            >
              <tab.icon className="w-4 h-4" />
              {tab.label}
              {tab.id === 'events' && stats && stats.unread_alerts > 0 && (
                <span className="ml-1 px-1.5 py-0.5 text-xs bg-red-500 text-white rounded-full">{stats.unread_alerts}</span>
              )}
            </button>
          ))}
        </nav>
      </div>

      {/* Rules Tab */}
      {activeTab === 'rules' && (
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
            <h3 className="font-semibold text-gray-900 dark:text-white">Protection Rules</h3>
            <button onClick={() => setShowRuleModal(true)} className="flex items-center gap-1 px-3 py-1.5 bg-blue-600 text-white rounded-lg text-sm hover:bg-blue-700">
              <Plus className="w-4 h-4" /> Add Rule
            </button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-700/50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Path</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Pattern</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Watching</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {rules.length === 0 ? (
                  <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-500">No protection rules configured</td></tr>
                ) : rules.map((r) => (
                  <tr key={r.id} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                    <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">{r.name}</td>
                    <td className="px-4 py-3 text-sm text-gray-500 font-mono">{r.path}</td>
                    <td className="px-4 py-3 text-sm text-gray-500 font-mono">{r.file_pattern}</td>
                    <td className="px-4 py-3">
                      <div className="flex gap-1">
                        {r.watch_create && <span className="px-1.5 py-0.5 text-xs bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400 rounded">C</span>}
                        {r.watch_modify && <span className="px-1.5 py-0.5 text-xs bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400 rounded">M</span>}
                        {r.watch_delete && <span className="px-1.5 py-0.5 text-xs bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400 rounded">D</span>}
                        {r.watch_permissions && <span className="px-1.5 py-0.5 text-xs bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400 rounded">P</span>}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <button onClick={() => toggleRule(r.id)} className="flex items-center gap-1">
                        {r.is_active ? <ToggleRight className="w-5 h-5 text-green-500" /> : <ToggleLeft className="w-5 h-5 text-gray-400" />}
                        <span className={`text-xs ${r.is_active ? 'text-green-600' : 'text-gray-400'}`}>{r.is_active ? 'Active' : 'Disabled'}</span>
                      </button>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button onClick={() => deleteRule(r.id)} className="text-red-600 hover:text-red-800"><Trash2 className="w-4 h-4" /></button>
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
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
            <h3 className="font-semibold text-gray-900 dark:text-white">Change Events</h3>
            <button onClick={markAllRead} className="flex items-center gap-1 px-3 py-1.5 bg-gray-100 dark:bg-gray-700 rounded-lg text-sm hover:bg-gray-200 dark:hover:bg-gray-600">
              <Eye className="w-4 h-4" /> Mark All Read
            </button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-700/50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">File</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Event</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Severity</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Details</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Time</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {events.length === 0 ? (
                  <tr><td colSpan={5} className="px-4 py-8 text-center text-gray-500">No change events detected</td></tr>
                ) : events.map((e) => (
                  <tr key={e.id} className={`hover:bg-gray-50 dark:hover:bg-gray-700/50 ${!e.is_read ? 'bg-blue-50/50 dark:bg-blue-900/10' : ''}`}>
                    <td className="px-4 py-3 text-sm font-mono text-gray-900 dark:text-white truncate max-w-[250px]">{e.file_path}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-1 text-xs rounded-full ${
                        e.event_type === 'created' ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' :
                        e.event_type === 'modified' ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400' :
                        e.event_type === 'deleted' ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400' :
                        'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400'
                      }`}>
                        {e.event_type}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-1 text-xs rounded-full ${severityColor(e.severity)}`}>{e.severity}</span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500 truncate max-w-[200px]">{e.details || '—'}</td>
                    <td className="px-4 py-3 text-sm text-gray-500">{new Date(e.created_at).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Quarantine Tab */}
      {activeTab === 'quarantine' && (
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700">
            <h3 className="font-semibold text-gray-900 dark:text-white">Quarantined Files</h3>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-700/50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Original Path</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Reason</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Size</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Date</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {quarantine.length === 0 ? (
                  <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-500">No quarantined files</td></tr>
                ) : quarantine.map((q) => (
                  <tr key={q.id} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                    <td className="px-4 py-3 text-sm font-mono text-gray-900 dark:text-white truncate max-w-[250px]">{q.original_path}</td>
                    <td className="px-4 py-3 text-sm text-gray-500 truncate max-w-[200px]">{q.reason}</td>
                    <td className="px-4 py-3 text-sm text-gray-500">{(q.file_size / 1024).toFixed(1)} KB</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-1 text-xs rounded-full ${q.restored_at ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' : 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'}`}>
                        {q.restored_at ? 'Restored' : 'Quarantined'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500">{new Date(q.created_at).toLocaleDateString()}</td>
                    <td className="px-4 py-3 text-right space-x-2">
                      {!q.restored_at && (
                        <button onClick={() => restoreQuarantine(q.id)} className="text-green-600 hover:text-green-800 text-sm">Restore</button>
                      )}
                      <button onClick={() => deleteQuarantine(q.id)} className="text-red-600 hover:text-red-800"><Trash2 className="w-4 h-4 inline" /></button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Create Rule Modal */}
      {showRuleModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-lg">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Add Protection Rule</h3>
            <div className="space-y-3">
              <input value={newRule.name} onChange={(e) => setNewRule({ ...newRule, name: e.target.value })} placeholder="Rule name" className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
              <input value={newRule.path} onChange={(e) => setNewRule({ ...newRule, path: e.target.value })} placeholder="/etc/nginx" className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
              <input value={newRule.file_pattern} onChange={(e) => setNewRule({ ...newRule, file_pattern: e.target.value })} placeholder="*.conf" className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
              <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input type="checkbox" checked={newRule.recursive} onChange={(e) => setNewRule({ ...newRule, recursive: e.target.checked })} /> Recursive
              </label>
              <div className="grid grid-cols-2 gap-2">
                {(['watch_create', 'watch_modify', 'watch_delete', 'watch_permissions'] as const).map((key) => (
                  <label key={key} className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                    <input type="checkbox" checked={newRule[key]} onChange={(e) => setNewRule({ ...newRule, [key]: e.target.checked })} />
                    {key.replace('watch_', '').charAt(0).toUpperCase() + key.replace('watch_', '').slice(1)}
                  </label>
                ))}
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-4">
              <button onClick={() => setShowRuleModal(false)} className="px-4 py-2 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">Cancel</button>
              <button onClick={createRule} className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700">Create</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
