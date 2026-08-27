'use client';

import { useState, useEffect } from 'react';
import { Mail, Plus, Trash2, RefreshCw, Server, Users, Globe, Send } from 'lucide-react';

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

const CARD = 'bg-white border border-gray-200 rounded-lg shadow-sm';
const CARD_HEADER = 'px-5 py-4 border-b border-gray-200 flex items-center justify-between gap-4';
const TH = 'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
const TD = 'px-4 py-3 text-sm text-gray-700';
const ROW = 'border-b border-gray-100 hover:bg-gray-50';
const INPUT =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 focus:outline-none';
const BTN_PRIMARY =
  'inline-flex items-center gap-2 rounded-md bg-blue-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-1';
const BTN_SECONDARY =
  'inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-1';
const BTN_DANGER =
  'inline-flex items-center gap-2 rounded-md bg-red-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-red-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500 focus-visible:ring-offset-1';
const ICON_DANGER =
  'rounded-md p-1 text-red-600 hover:bg-red-50 hover:text-red-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500';
const BADGE = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium';
const MODAL_BACKDROP = 'fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40 p-4';
const MODAL_PANEL = 'w-full max-w-md rounded-lg border border-gray-200 bg-white shadow-lg';

const STAT_TONES: Record<string, { wrap: string; icon: string }> = {
  blue: { wrap: 'bg-blue-50', icon: 'text-blue-600' },
  emerald: { wrap: 'bg-emerald-50', icon: 'text-emerald-600' },
  sky: { wrap: 'bg-sky-50', icon: 'text-sky-600' },
  amber: { wrap: 'bg-amber-50', icon: 'text-amber-600' },
};

function formatDate(value?: string): string {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleDateString();
}

export default function MailServerPage() {
  const [stats, setStats] = useState<MailStats | null>(null);
  const [domains, setDomains] = useState<MailDomain[]>([]);
  const [accounts, setAccounts] = useState<MailAccount[]>([]);
  const [aliases, setAliases] = useState<MailAlias[]>([]);
  const [queue, setQueue] = useState<MailQueueItem[]>([]);
  const [activeTab, setActiveTab] = useState('domains');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [showDomainModal, setShowDomainModal] = useState(false);
  const [showAccountModal, setShowAccountModal] = useState(false);
  const [showAliasModal, setShowAliasModal] = useState(false);
  const [newDomain, setNewDomain] = useState('');
  const [newAccount, setNewAccount] = useState({ domain_id: '', email: '', password: '', quota_mb: 1024 });
  const [newAlias, setNewAlias] = useState({ domain_id: '', source: '', destination: '' });

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
    setError('');
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
      if (statsRes.ok) { const d = await statsRes.json(); setStats(d?.stats ?? null); }
      if (domainsRes.ok) { const d = await domainsRes.json(); setDomains(Array.isArray(d?.domains) ? d.domains : []); }
      if (accountsRes.ok) { const d = await accountsRes.json(); setAccounts(Array.isArray(d?.accounts) ? d.accounts : []); }
      if (aliasesRes.ok) { const d = await aliasesRes.json(); setAliases(Array.isArray(d?.aliases) ? d.aliases : []); }
      if (queueRes.ok) { const d = await queueRes.json(); setQueue(Array.isArray(d?.queue) ? d.queue : []); }
    } catch (e) {
      console.error(e);
      setError('Unable to load mail server data. Please try again.');
    }
    setLoading(false);
  };

  useEffect(() => { fetchAll(); }, []);

  const createDomain = async () => {
    const token = getToken();
    try {
      await fetch('/api/v1/mail-server/domains', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ domain: newDomain }),
      });
      setShowDomainModal(false);
      setNewDomain('');
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Unable to create domain. Please try again.');
    }
  };

  const deleteDomain = async (id: string) => {
    if (!confirm('Delete this domain? All accounts under it will be removed.')) return;
    const token = getToken();
    try {
      await fetch(`/api/v1/mail-server/domains/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Unable to delete domain. Please try again.');
    }
  };

  const createAccount = async () => {
    const token = getToken();
    try {
      await fetch('/api/v1/mail-server/accounts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify(newAccount),
      });
      setShowAccountModal(false);
      setNewAccount({ domain_id: '', email: '', password: '', quota_mb: 1024 });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Unable to create account. Please try again.');
    }
  };

  const deleteAccount = async (id: string) => {
    if (!confirm('Delete this email account?')) return;
    const token = getToken();
    try {
      await fetch(`/api/v1/mail-server/accounts/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Unable to delete account. Please try again.');
    }
  };

  const createAlias = async () => {
    const token = getToken();
    try {
      await fetch('/api/v1/mail-server/aliases', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify(newAlias),
      });
      setShowAliasModal(false);
      setNewAlias({ domain_id: '', source: '', destination: '' });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Unable to create alias. Please try again.');
    }
  };

  const deleteAlias = async (id: string) => {
    const token = getToken();
    try {
      await fetch(`/api/v1/mail-server/aliases/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${token}` } });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Unable to delete alias. Please try again.');
    }
  };

  const flushQueue = async () => {
    if (!confirm('Flush all failed messages from queue?')) return;
    const token = getToken();
    try {
      await fetch('/api/v1/mail-server/queue/flush', { method: 'POST', headers: { Authorization: `Bearer ${token}` } });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Unable to flush the queue. Please try again.');
    }
  };

  const deleteQueueItem = async (id: string) => {
    const t = getToken();
    try {
      await fetch(`/api/v1/mail-server/queue/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${t}` } });
      fetchAll();
    } catch (e) {
      console.error(e);
      setError('Unable to remove the queued message. Please try again.');
    }
  };

  const tabs = [
    { id: 'domains', label: 'Domains', icon: Globe },
    { id: 'accounts', label: 'Accounts', icon: Users },
    { id: 'aliases', label: 'Aliases', icon: Mail },
    { id: 'queue', label: 'Queue', icon: Send },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold text-gray-900">
            <Server className="w-5 h-5 text-gray-500" /> Mail Server
          </h1>
          <p className="mt-1 text-sm text-gray-600">Manage mail domains, accounts, aliases and queue</p>
        </div>
        <button type="button" onClick={fetchAll} className={BTN_SECONDARY}>
          <RefreshCw className="w-4 h-4" /> Refresh
        </button>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {/* Stats Cards */}
      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          {[
            { label: 'Domains', value: stats.total_domains ?? 0, icon: Globe, color: 'blue' },
            { label: 'Accounts', value: stats.total_accounts ?? 0, icon: Users, color: 'emerald' },
            { label: 'Sent Today', value: stats.sent_today ?? 0, icon: Send, color: 'sky' },
            { label: 'Queue', value: stats.queue_size ?? 0, icon: Mail, color: 'amber' },
          ].map((s) => {
            const tone = STAT_TONES[s.color] ?? STAT_TONES.blue;
            return (
              <div key={s.label} className={`${CARD} p-4`}>
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{s.label}</p>
                    <p className="mt-1 text-2xl font-semibold text-gray-900">{s.value}</p>
                  </div>
                  <div className={`rounded-md p-2.5 ${tone.wrap}`}>
                    <s.icon className={`w-5 h-5 ${tone.icon}`} />
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="Mail server sections">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              aria-current={activeTab === tab.id ? 'page' : undefined}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 border-b-2 px-1 pb-3 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                activeTab === tab.id
                  ? 'border-blue-600 text-blue-700'
                  : 'border-transparent text-gray-600 hover:border-gray-300 hover:text-gray-900'
              }`}
            >
              <tab.icon className="w-4 h-4" />
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {loading && (
        <div className={`${CARD} px-5 py-8 text-center text-sm text-gray-500`}>Loading…</div>
      )}

      {/* Domains Tab */}
      {!loading && activeTab === 'domains' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Mail Domains</h2>
            <button type="button" onClick={() => setShowDomainModal(true)} className={BTN_PRIMARY}>
              <Plus className="w-4 h-4" /> Add Domain
            </button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  <th className={TH}>Domain</th>
                  <th className={TH}>Status</th>
                  <th className={TH}>DKIM</th>
                  <th className={TH}>Created</th>
                  <th className={`${TH} text-right`}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {domains.length === 0 ? (
                  <tr><td colSpan={5} className="px-4 py-10 text-center text-sm text-gray-500">No domains configured</td></tr>
                ) : domains.map((d) => (
                  <tr key={d.id} className={ROW}>
                    <td className={`${TD} font-medium text-gray-900`}>{d.domain}</td>
                    <td className="px-4 py-3">
                      <span className={`${BADGE} ${d.is_active ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'}`}>
                        {d.is_active ? 'Active' : 'Inactive'}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`${BADGE} ${d.dkim_enabled ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-600'}`}>
                        {d.dkim_enabled ? 'Enabled' : 'Disabled'}
                      </span>
                    </td>
                    <td className={TD} suppressHydrationWarning>{formatDate(d.created_at)}</td>
                    <td className="px-4 py-3 text-right">
                      <button type="button" aria-label={`Delete domain ${d.domain}`} onClick={() => deleteDomain(d.id)} className={ICON_DANGER}>
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Accounts Tab */}
      {!loading && activeTab === 'accounts' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Email Accounts</h2>
            <button type="button" onClick={() => setShowAccountModal(true)} className={BTN_PRIMARY}>
              <Plus className="w-4 h-4" /> Add Account
            </button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  <th className={TH}>Email</th>
                  <th className={TH}>Status</th>
                  <th className={TH}>Quota</th>
                  <th className={TH}>Forward To</th>
                  <th className={TH}>Auto Reply</th>
                  <th className={`${TH} text-right`}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {accounts.length === 0 ? (
                  <tr><td colSpan={6} className="px-4 py-10 text-center text-sm text-gray-500">No email accounts</td></tr>
                ) : accounts.map((a) => (
                  <tr key={a.id} className={ROW}>
                    <td className={`${TD} font-medium text-gray-900`}>{a.email}</td>
                    <td className="px-4 py-3">
                      <span className={`${BADGE} ${a.is_active ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'}`}>
                        {a.is_active ? 'Active' : 'Disabled'}
                      </span>
                    </td>
                    <td className={TD}>{a.used_mb ?? 0}/{a.quota_mb ?? 0} MB</td>
                    <td className={TD}>{a.forward_to || '—'}</td>
                    <td className="px-4 py-3">
                      <span className={`${BADGE} ${a.auto_reply ? 'bg-blue-50 text-blue-700' : 'bg-gray-100 text-gray-600'}`}>
                        {a.auto_reply ? 'On' : 'Off'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button type="button" aria-label={`Delete account ${a.email}`} onClick={() => deleteAccount(a.id)} className={ICON_DANGER}>
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Aliases Tab */}
      {!loading && activeTab === 'aliases' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Mail Aliases</h2>
            <button type="button" onClick={() => setShowAliasModal(true)} className={BTN_PRIMARY}>
              <Plus className="w-4 h-4" /> Add Alias
            </button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  <th className={TH}>Source</th>
                  <th className={TH}>Destination</th>
                  <th className={TH}>Status</th>
                  <th className={`${TH} text-right`}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {aliases.length === 0 ? (
                  <tr><td colSpan={4} className="px-4 py-10 text-center text-sm text-gray-500">No aliases configured</td></tr>
                ) : aliases.map((a) => (
                  <tr key={a.id} className={ROW}>
                    <td className={`${TD} font-medium text-gray-900`}>{a.source}</td>
                    <td className={TD}>{a.destination}</td>
                    <td className="px-4 py-3">
                      <span className={`${BADGE} ${a.is_active ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'}`}>
                        {a.is_active ? 'Active' : 'Inactive'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button type="button" aria-label={`Delete alias ${a.source}`} onClick={() => deleteAlias(a.id)} className={ICON_DANGER}>
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Queue Tab */}
      {!loading && activeTab === 'queue' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Mail Queue</h2>
            <button type="button" onClick={flushQueue} className={BTN_DANGER}>
              <Trash2 className="w-4 h-4" /> Flush Failed
            </button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  <th className={TH}>From</th>
                  <th className={TH}>To</th>
                  <th className={TH}>Subject</th>
                  <th className={TH}>Status</th>
                  <th className={TH}>Retries</th>
                  <th className={`${TH} text-right`}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {queue.length === 0 ? (
                  <tr><td colSpan={6} className="px-4 py-10 text-center text-sm text-gray-500">Queue is empty</td></tr>
                ) : queue.map((q) => (
                  <tr key={q.id} className={ROW}>
                    <td className={`${TD} text-gray-900`}>{q.from || '—'}</td>
                    <td className={TD}>{q.to || '—'}</td>
                    <td className={`${TD} max-w-[200px] truncate`}>{q.subject || '—'}</td>
                    <td className="px-4 py-3">
                      <span className={`${BADGE} ${
                        q.status === 'sent' ? 'bg-emerald-50 text-emerald-700' :
                        q.status === 'failed' ? 'bg-red-50 text-red-700' :
                        q.status === 'deferred' ? 'bg-amber-50 text-amber-700' :
                        'bg-blue-50 text-blue-700'
                      }`}>
                        {q.status || 'unknown'}
                      </span>
                    </td>
                    <td className={TD}>{q.retry_count ?? 0}</td>
                    <td className="px-4 py-3 text-right">
                      <button type="button" aria-label="Remove queued message" onClick={() => deleteQueueItem(q.id)} className={ICON_DANGER}>
                        <Trash2 className="w-4 h-4" />
                      </button>
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
        <div className={MODAL_BACKDROP}>
          <div className={MODAL_PANEL}>
            <div className="border-b border-gray-200 px-5 py-4">
              <h2 className="text-sm font-semibold text-gray-900">Add Mail Domain</h2>
            </div>
            <div className="p-5">
              <label htmlFor="mail-domain" className="mb-1.5 block text-sm font-medium text-gray-700">Domain</label>
              <input id="mail-domain" value={newDomain} onChange={(e) => setNewDomain(e.target.value)} placeholder="example.com" className={INPUT} />
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              <button type="button" onClick={() => setShowDomainModal(false)} className={BTN_SECONDARY}>Cancel</button>
              <button type="button" onClick={createDomain} className={BTN_PRIMARY}>Create</button>
            </div>
          </div>
        </div>
      )}

      {/* Create Account Modal */}
      {showAccountModal && (
        <div className={MODAL_BACKDROP}>
          <div className={MODAL_PANEL}>
            <div className="border-b border-gray-200 px-5 py-4">
              <h2 className="text-sm font-semibold text-gray-900">Add Email Account</h2>
            </div>
            <div className="space-y-4 p-5">
              <div>
                <label htmlFor="account-domain" className="mb-1.5 block text-sm font-medium text-gray-700">Domain</label>
                <select id="account-domain" value={newAccount.domain_id} onChange={(e) => setNewAccount({ ...newAccount, domain_id: e.target.value })} className={INPUT}>
                  <option value="">Select Domain</option>
                  {domains.map((d) => <option key={d.id} value={d.id}>{d.domain}</option>)}
                </select>
              </div>
              <div>
                <label htmlFor="account-email" className="mb-1.5 block text-sm font-medium text-gray-700">Email</label>
                <input id="account-email" value={newAccount.email} onChange={(e) => setNewAccount({ ...newAccount, email: e.target.value })} placeholder="user@example.com" className={INPUT} />
              </div>
              <div>
                <label htmlFor="account-password" className="mb-1.5 block text-sm font-medium text-gray-700">Password</label>
                <input id="account-password" type="password" value={newAccount.password} onChange={(e) => setNewAccount({ ...newAccount, password: e.target.value })} placeholder="Password" className={INPUT} />
              </div>
              <div>
                <label htmlFor="account-quota" className="mb-1.5 block text-sm font-medium text-gray-700">Quota (MB)</label>
                <input id="account-quota" type="number" value={newAccount.quota_mb} onChange={(e) => setNewAccount({ ...newAccount, quota_mb: parseInt(e.target.value) || 0 })} placeholder="Quota (MB)" className={INPUT} />
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              <button type="button" onClick={() => setShowAccountModal(false)} className={BTN_SECONDARY}>Cancel</button>
              <button type="button" onClick={createAccount} className={BTN_PRIMARY}>Create</button>
            </div>
          </div>
        </div>
      )}

      {/* Create Alias Modal */}
      {showAliasModal && (
        <div className={MODAL_BACKDROP}>
          <div className={MODAL_PANEL}>
            <div className="border-b border-gray-200 px-5 py-4">
              <h2 className="text-sm font-semibold text-gray-900">Add Mail Alias</h2>
            </div>
            <div className="space-y-4 p-5">
              <div>
                <label htmlFor="alias-domain" className="mb-1.5 block text-sm font-medium text-gray-700">Domain</label>
                <select id="alias-domain" value={newAlias.domain_id} onChange={(e) => setNewAlias({ ...newAlias, domain_id: e.target.value })} className={INPUT}>
                  <option value="">Select Domain</option>
                  {domains.map((d) => <option key={d.id} value={d.id}>{d.domain}</option>)}
                </select>
              </div>
              <div>
                <label htmlFor="alias-source" className="mb-1.5 block text-sm font-medium text-gray-700">Source</label>
                <input id="alias-source" value={newAlias.source} onChange={(e) => setNewAlias({ ...newAlias, source: e.target.value })} placeholder="info@example.com" className={INPUT} />
              </div>
              <div>
                <label htmlFor="alias-destination" className="mb-1.5 block text-sm font-medium text-gray-700">Destination</label>
                <input id="alias-destination" value={newAlias.destination} onChange={(e) => setNewAlias({ ...newAlias, destination: e.target.value })} placeholder="user1@example.com, user2@example.com" className={INPUT} />
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              <button type="button" onClick={() => setShowAliasModal(false)} className={BTN_SECONDARY}>Cancel</button>
              <button type="button" onClick={createAlias} className={BTN_PRIMARY}>Create</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
