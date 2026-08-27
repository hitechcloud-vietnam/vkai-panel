'use client';

import { useState, useEffect } from 'react';
import { Mail, Plus, Trash2, RefreshCw, Shield, Settings, Server, Users, Globe, Send, Filter } from 'lucide-react';

interface MailDomain {
  id: string;
  domain: string;
  is_verified: boolean;
  is_active: boolean;
  dkim_enabled: boolean;
  created_at: string;
}

interface MailAccount {
  id: string;
  email: string;
  domain_id: string;
  quota_mb: number;
  used_mb: number;
  is_active: boolean;
  forward_to: string;
  auto_reply: boolean;
  created_at: string;
}

interface MailAlias {
  id: string;
  source: string;
  destination: string;
  is_active: boolean;
  created_at: string;
}

interface MailQueueItem {
  id: string;
  from: string;
  to: string;
  subject: string;
  status: string;
  retry_count: number;
  created_at: string;
}

interface MailStats {
  total_domains: number;
  total_accounts: number;
  total_aliases: number;
  queue_size: number;
  sent_today: number;
  failed_today: number;
  storage_used_mb: number;
}

export default function MailServerPage() {
  const [stats, setStats] = useState<MailStats | null>(null);
  const [domains, setDomains] = useState<MailDomain[]>([]);
  const [accounts, setAccounts] = useState<MailAccount[]>([]);
  const [aliases, setAliases] = useState<MailAlias[]>([]);
  const [queue, setQueue] = useState<MailQueueItem[]>([]);
  const [activeTab, setActiveTab] = useState('domains');
  const [loading, setLoading] = useState(true);
  const [showDomainModal, setShowDomainModal] = useState(false);
  const [showAccountModal, setShowAccountModal] = useState(false);
  const [showAliasModal, setShowAliasModal] = useState(false);
  const [newDomain, setNewDomain] = useState('');
  const [newAccount, setNewAccount] = useState({ domain_id: '', email: '', password: '', quota_mb: 1024 });
  const [newAlias, setNewAlias] = useState({ domain_id: '', source: '', destination: '' });

  const getToken = () => localStorage.getItem('access_token') || '';

  const fetchAll = async () => {
    setLoading(true);
    const token = getToken();
    const headers = { Authorization: `Bearer ${token}` };
    try {
      const [statsRes, domainsRes, accountsRes, aliasesRes, queueRes] = await Promise.all([
        fetch('/api/v1/mail-server/stats', { headers }),
        fetch('/api/v1/mail-server/domains', { headers }),
        fetch('/api/v1/mail-server/accounts', { headers }),
        fetch('/api/v1/mail-server/aliases', { headers }),
        fetch('/api/v1/mail-server/queue', { headers }),
      ]);
      if (statsRes.ok) { const d = await statsRes.json(); setStats(d.stats); }
      if (domainsRes.ok) { const d = await domainsRes.json(); setDomains(d.domains || []); }
      if (accountsRes.ok) { const d = await accountsRes.json(); setAccounts(d.accounts || []); }
      if (aliasesRes.ok) { const d = await aliasesRes.json(); setAliases(d.aliases || []); }
      if (queueRes.ok) { const d = await queueRes.json(); setQueue(d.queue || []); }
    } catch (e) { console.error(e); }
    setLoading(false);
  };

  useEffect(() => { fetchAll(); }, []);

  const createDomain = async () => {
    const token = getToken();
    await fetch('/api/v1/mail-server/domains', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({ domain: newDomain }),
    });
    setShowDomainModal(false);
    setNewDomain('');
    fetchAll();
  };

  const deleteDomain = async (id: string) => {
    if (!confirm('Delete this domain? All accounts under it will be removed.')) return;
    const token = getToken();
    await fetch(`/api/v1/mail-server/domains/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
    fetchAll();
  };

  const createAccount = async () => {
    const token = getToken();
    await fetch('/api/v1/mail-server/accounts', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify(newAccount),
    });
    setShowAccountModal(false);
    setNewAccount({ domain_id: '', email: '', password: '', quota_mb: 1024 });
    fetchAll();
  };

  const deleteAccount = async (id: string) => {
    if (!confirm('Delete this email account?')) return;
    const token = getToken();
    await fetch(`/api/v1/mail-server/accounts/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
    fetchAll();
  };

  const createAlias = async () => {
    const token = getToken();
    await fetch('/api/v1/mail-server/aliases', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify(newAlias),
    });
    setShowAliasModal(false);
    setNewAlias({ domain_id: '', source: '', destination: '' });
    fetchAll();
  };

  const deleteAlias = async (id: string) => {
    const token = getToken();
    await fetch(`/api/v1/mail-server/aliases/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
    fetchAll();
  };

  const flushQueue = async () => {
    if (!confirm('Flush all failed messages from queue?')) return;
    const token = getToken();
    await fetch('/api/v1/mail-server/queue/flush', { method: 'POST', headers: { Authorization: `Bearer ${token}` } });
    fetchAll();
  };

  const tabs = [
    { id: 'domains', label: 'Domains', icon: Globe },
    { id: 'accounts', label: 'Accounts', icon: Users },
    { id: 'aliases', label: 'Aliases', icon: Mail },
    { id: 'queue', label: 'Queue', icon: Send },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
            <Server className="w-6 h-6" /> Mail Server
          </h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">Manage mail domains, accounts, aliases and queue</p>
        </div>
        <button onClick={fetchAll} className="flex items-center gap-2 px-4 py-2 bg-gray-100 dark:bg-gray-700 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600">
          <RefreshCw className="w-4 h-4" /> Refresh
        </button>
      </div>

      {/* Stats Cards */}
      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          {[
            { label: 'Domains', value: stats.total_domains, icon: Globe, color: 'blue' },
            { label: 'Accounts', value: stats.total_accounts, icon: Users, color: 'green' },
            { label: 'Sent Today', value: stats.sent_today, icon: Send, color: 'purple' },
            { label: 'Queue', value: stats.queue_size, icon: Mail, color: 'orange' },
          ].map((s) => (
            <div key={s.label} className="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-200 dark:border-gray-700">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-gray-500 dark:text-gray-400">{s.label}</p>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white">{s.value}</p>
                </div>
                <div className={`p-3 rounded-lg bg-${s.color}-100 dark:bg-${s.color}-900/20`}>
                  <s.icon className={`w-5 h-5 text-${s.color}-600 dark:text-${s.color}-400`} />
                </div>
              </div>
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
            </button>
          ))}
        </nav>
      </div>

      {/* Domains Tab */}
      {activeTab === 'domains' && (
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
            <h3 className="font-semibold text-gray-900 dark:text-white">Mail Domains</h3>
            <button onClick={() => setShowDomainModal(true)} className="flex items-center gap-1 px-3 py-1.5 bg-blue-600 text-white rounded-lg text-sm hover:bg-blue-700">
              <Plus className="w-4 h-4" /> Add Domain
            </button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-700/50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Domain</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">DKIM</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Created</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {domains.length === 0 ? (
                  <tr><td colSpan={5} className="px-4 py-8 text-center text-gray-500">No domains configured</td></tr>
                ) : domains.map((d) => (
                  <tr key={d.id} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                    <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">{d.domain}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-1 text-xs rounded-full ${d.is_active ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' : 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'}`}>
                        {d.is_active ? 'Active' : 'Inactive'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500">{d.dkim_enabled ? '✅ Enabled' : '❌ Disabled'}</td>
                    <td className="px-4 py-3 text-sm text-gray-500">{new Date(d.created_at).toLocaleDateString()}</td>
                    <td className="px-4 py-3 text-right">
                      <button onClick={() => deleteDomain(d.id)} className="text-red-600 hover:text-red-800"><Trash2 className="w-4 h-4" /></button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Accounts Tab */}
      {activeTab === 'accounts' && (
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
            <h3 className="font-semibold text-gray-900 dark:text-white">Email Accounts</h3>
            <button onClick={() => setShowAccountModal(true)} className="flex items-center gap-1 px-3 py-1.5 bg-blue-600 text-white rounded-lg text-sm hover:bg-blue-700">
              <Plus className="w-4 h-4" /> Add Account
            </button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-700/50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Email</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Quota</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Forward To</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Auto Reply</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {accounts.length === 0 ? (
                  <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-500">No email accounts</td></tr>
                ) : accounts.map((a) => (
                  <tr key={a.id} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                    <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">{a.email}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-1 text-xs rounded-full ${a.is_active ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' : 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'}`}>
                        {a.is_active ? 'Active' : 'Disabled'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500">{a.used_mb}/{a.quota_mb} MB</td>
                    <td className="px-4 py-3 text-sm text-gray-500">{a.forward_to || '—'}</td>
                    <td className="px-4 py-3 text-sm text-gray-500">{a.auto_reply ? '✅' : '—'}</td>
                    <td className="px-4 py-3 text-right">
                      <button onClick={() => deleteAccount(a.id)} className="text-red-600 hover:text-red-800"><Trash2 className="w-4 h-4" /></button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Aliases Tab */}
      {activeTab === 'aliases' && (
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
            <h3 className="font-semibold text-gray-900 dark:text-white">Mail Aliases</h3>
            <button onClick={() => setShowAliasModal(true)} className="flex items-center gap-1 px-3 py-1.5 bg-blue-600 text-white rounded-lg text-sm hover:bg-blue-700">
              <Plus className="w-4 h-4" /> Add Alias
            </button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-700/50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Source</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Destination</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {aliases.length === 0 ? (
                  <tr><td colSpan={4} className="px-4 py-8 text-center text-gray-500">No aliases configured</td></tr>
                ) : aliases.map((a) => (
                  <tr key={a.id} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                    <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">{a.source}</td>
                    <td className="px-4 py-3 text-sm text-gray-500">{a.destination}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-1 text-xs rounded-full ${a.is_active ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' : 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'}`}>
                        {a.is_active ? 'Active' : 'Inactive'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button onClick={() => deleteAlias(a.id)} className="text-red-600 hover:text-red-800"><Trash2 className="w-4 h-4" /></button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Queue Tab */}
      {activeTab === 'queue' && (
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
            <h3 className="font-semibold text-gray-900 dark:text-white">Mail Queue</h3>
            <button onClick={flushQueue} className="flex items-center gap-1 px-3 py-1.5 bg-red-600 text-white rounded-lg text-sm hover:bg-red-700">
              <Trash2 className="w-4 h-4" /> Flush Failed
            </button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-700/50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">From</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">To</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Subject</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Retries</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {queue.length === 0 ? (
                  <tr><td colSpan={6} className="px-4 py-8 text-center text-gray-500">Queue is empty</td></tr>
                ) : queue.map((q) => (
                  <tr key={q.id} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                    <td className="px-4 py-3 text-sm text-gray-900 dark:text-white">{q.from}</td>
                    <td className="px-4 py-3 text-sm text-gray-500">{q.to}</td>
                    <td className="px-4 py-3 text-sm text-gray-500 truncate max-w-[200px]">{q.subject}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-1 text-xs rounded-full ${
                        q.status === 'sent' ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' :
                        q.status === 'failed' ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400' :
                        q.status === 'deferred' ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400' :
                        'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400'
                      }`}>
                        {q.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500">{q.retry_count}</td>
                    <td className="px-4 py-3 text-right">
                      <button onClick={async () => { const t = getToken(); await fetch(`/api/v1/mail-server/queue/${q.id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${t}` } }); fetchAll(); }} className="text-red-600 hover:text-red-800"><Trash2 className="w-4 h-4" /></button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Create Domain Modal */}
      {showDomainModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-md">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Add Mail Domain</h3>
            <input value={newDomain} onChange={(e) => setNewDomain(e.target.value)} placeholder="example.com" className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white mb-4" />
            <div className="flex justify-end gap-2">
              <button onClick={() => setShowDomainModal(false)} className="px-4 py-2 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">Cancel</button>
              <button onClick={createDomain} className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700">Create</button>
            </div>
          </div>
        </div>
      )}

      {/* Create Account Modal */}
      {showAccountModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-md">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Add Email Account</h3>
            <div className="space-y-3">
              <select value={newAccount.domain_id} onChange={(e) => setNewAccount({ ...newAccount, domain_id: e.target.value })} className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white">
                <option value="">Select Domain</option>
                {domains.map((d) => <option key={d.id} value={d.id}>{d.domain}</option>)}
              </select>
              <input value={newAccount.email} onChange={(e) => setNewAccount({ ...newAccount, email: e.target.value })} placeholder="user@example.com" className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
              <input type="password" value={newAccount.password} onChange={(e) => setNewAccount({ ...newAccount, password: e.target.value })} placeholder="Password" className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
              <input type="number" value={newAccount.quota_mb} onChange={(e) => setNewAccount({ ...newAccount, quota_mb: parseInt(e.target.value) })} placeholder="Quota (MB)" className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
            </div>
            <div className="flex justify-end gap-2 mt-4">
              <button onClick={() => setShowAccountModal(false)} className="px-4 py-2 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">Cancel</button>
              <button onClick={createAccount} className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700">Create</button>
            </div>
          </div>
        </div>
      )}

      {/* Create Alias Modal */}
      {showAliasModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-md">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Add Mail Alias</h3>
            <div className="space-y-3">
              <select value={newAlias.domain_id} onChange={(e) => setNewAlias({ ...newAlias, domain_id: e.target.value })} className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white">
                <option value="">Select Domain</option>
                {domains.map((d) => <option key={d.id} value={d.id}>{d.domain}</option>)}
              </select>
              <input value={newAlias.source} onChange={(e) => setNewAlias({ ...newAlias, source: e.target.value })} placeholder="info@example.com" className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
              <input value={newAlias.destination} onChange={(e) => setNewAlias({ ...newAlias, destination: e.target.value })} placeholder="user1@example.com, user2@example.com" className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
            </div>
            <div className="flex justify-end gap-2 mt-4">
              <button onClick={() => setShowAliasModal(false)} className="px-4 py-2 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg">Cancel</button>
              <button onClick={createAlias} className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700">Create</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
