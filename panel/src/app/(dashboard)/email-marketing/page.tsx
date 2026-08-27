'use client';

import { useState, useEffect } from 'react';
import {
  Mail, Plus, Send, Pause, Trash2, RefreshCw, Search,
  Users, FileText, Eye, MousePointer, AlertTriangle,
  X, Inbox
} from 'lucide-react';

interface Campaign {
  id: string;
  name: string;
  subject: string;
  status: string;
  from_name: string;
  from_email: string;
  total_recipients: number;
  sent_count: number;
  open_count: number;
  click_count: number;
  bounce_count: number;
  tags: string[];
  created_at: string;
  scheduled_at: string | null;
  sent_at: string | null;
}

interface Contact {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  status: string;
  source: string;
  tags: string[];
  created_at: string;
}

interface EmailList {
  id: string;
  name: string;
  description: string;
  contact_count: number;
  double_opt_in: boolean;
  created_at: string;
}

interface EmailTemplate {
  id: string;
  name: string;
  subject: string;
  category: string;
  created_at: string;
}

interface EmailStats {
  total_campaigns: number;
  total_contacts: number;
  total_lists: number;
  total_sent: number;
  total_opened: number;
  total_clicked: number;
  total_bounced: number;
  avg_open_rate: number;
  avg_click_rate: number;
  avg_bounce_rate: number;
}

const CARD = 'bg-white border border-gray-200 rounded-lg shadow-sm';
const CARD_HEADER = 'px-5 py-4 border-b border-gray-200';
const INPUT =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus:outline-none';
const LABEL = 'mb-1.5 block text-sm font-medium text-gray-700';
const BTN_PRIMARY =
  'inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1';
const BTN_SECONDARY =
  'inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1';
const BADGE = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium';
const ICON_DANGER =
  'rounded-md p-1.5 text-red-600 hover:bg-red-50 hover:text-red-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500';

function toPercent(value?: number): string {
  const n = typeof value === 'number' && Number.isFinite(value) ? value : 0;
  return `${n.toFixed(1)}%`;
}

export default function EmailMarketingPage() {
  const [activeTab, setActiveTab] = useState<'campaigns' | 'contacts' | 'lists' | 'templates'>('campaigns');
  const [stats, setStats] = useState<EmailStats | null>(null);
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [lists, setLists] = useState<EmailList[]>([]);
  const [templates, setTemplates] = useState<EmailTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [createType, setCreateType] = useState<'campaign' | 'contact' | 'list' | 'template'>('campaign');
  const [searchTerm, setSearchTerm] = useState('');

  // Campaign form
  const [campaignForm, setCampaignForm] = useState({
    name: '', subject: '', html_content: '', from_name: '', from_email: '', reply_to: '', tags: [] as string[]
  });
  // Contact form
  const [contactForm, setContactForm] = useState({
    email: '', first_name: '', last_name: '', tags: [] as string[]
  });
  // List form
  const [listForm, setListForm] = useState({ name: '', description: '', double_opt_in: false });
  // Template form
  const [templateForm, setTemplateForm] = useState({ name: '', subject: '', html_content: '', category: '' });

  const getToken = () => {
    if (typeof window === 'undefined') return '';
    try {
      return localStorage.getItem('token') || '';
    } catch {
      return '';
    }
  };

  const fetchStats = async () => {
    try {
      const res = await fetch('/api/v1/email-marketing/stats', {
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) {
        const data = await res.json();
        setStats(data?.stats ?? null);
      }
    } catch (e) { console.error(e); setError('Unable to load email marketing statistics.'); }
  };

  const fetchCampaigns = async () => {
    try {
      const res = await fetch('/api/v1/email-marketing/campaigns?limit=100&offset=0', {
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) {
        const data = await res.json();
        setCampaigns(Array.isArray(data?.campaigns) ? data.campaigns : []);
      }
    } catch (e) { console.error(e); setError('Unable to load campaigns.'); }
  };

  const fetchContacts = async () => {
    try {
      const res = await fetch(`/api/v1/email-marketing/contacts?limit=100&offset=0&search=${searchTerm}`, {
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) {
        const data = await res.json();
        setContacts(Array.isArray(data?.contacts) ? data.contacts : []);
      }
    } catch (e) { console.error(e); setError('Unable to load contacts.'); }
  };

  const fetchLists = async () => {
    try {
      const res = await fetch('/api/v1/email-marketing/lists', {
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) {
        const data = await res.json();
        setLists(Array.isArray(data?.lists) ? data.lists : []);
      }
    } catch (e) { console.error(e); setError('Unable to load mailing lists.'); }
  };

  const fetchTemplates = async () => {
    try {
      const res = await fetch('/api/v1/email-marketing/templates', {
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) {
        const data = await res.json();
        setTemplates(Array.isArray(data?.templates) ? data.templates : []);
      }
    } catch (e) { console.error(e); setError('Unable to load templates.'); }
  };

  const refreshAll = () => {
    setError('');
    fetchStats();
    fetchCampaigns();
    fetchContacts();
    fetchLists();
    fetchTemplates();
  };

  useEffect(() => {
    setLoading(true);
    Promise.all([fetchStats(), fetchCampaigns(), fetchContacts(), fetchLists(), fetchTemplates()])
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { fetchContacts(); }, [searchTerm]);

  const handleCreate = async () => {
    let url = '';
    let body = {};
    switch (createType) {
      case 'campaign':
        url = '/api/v1/email-marketing/campaigns';
        body = campaignForm;
        break;
      case 'contact':
        url = '/api/v1/email-marketing/contacts';
        body = contactForm;
        break;
      case 'list':
        url = '/api/v1/email-marketing/lists';
        body = listForm;
        break;
      case 'template':
        url = '/api/v1/email-marketing/templates';
        body = templateForm;
        break;
    }
    try {
      const res = await fetch(url, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${getToken()}`, 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });
      if (res.ok) {
        setShowCreateModal(false);
        fetchStats();
        fetchCampaigns();
        fetchContacts();
        fetchLists();
        fetchTemplates();
        // Reset forms
        setCampaignForm({ name: '', subject: '', html_content: '', from_name: '', from_email: '', reply_to: '', tags: [] });
        setContactForm({ email: '', first_name: '', last_name: '', tags: [] });
        setListForm({ name: '', description: '', double_opt_in: false });
        setTemplateForm({ name: '', subject: '', html_content: '', category: '' });
      } else {
        setError('The server rejected the request. Please review the form and try again.');
      }
    } catch (e) { console.error(e); setError('Unable to save. Please try again.'); }
  };

  const handleDelete = async (type: string, id: string) => {
    if (!confirm('Are you sure you want to delete this item?')) return;
    try {
      const res = await fetch(`/api/v1/email-marketing/${type}/${id}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) {
        fetchStats();
        fetchCampaigns();
        fetchContacts();
        fetchLists();
        fetchTemplates();
      }
    } catch (e) { console.error(e); setError('Unable to delete. Please try again.'); }
  };

  const handleSendCampaign = async (id: string) => {
    try {
      const res = await fetch(`/api/v1/email-marketing/campaigns/${id}/send`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) fetchCampaigns();
    } catch (e) { console.error(e); setError('Unable to send the campaign.'); }
  };

  const handlePauseCampaign = async (id: string) => {
    try {
      const res = await fetch(`/api/v1/email-marketing/campaigns/${id}/pause`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) fetchCampaigns();
    } catch (e) { console.error(e); setError('Unable to pause the campaign.'); }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'active': case 'sent': case 'delivered': return 'bg-emerald-50 text-emerald-700';
      case 'draft': case 'queued': return 'bg-gray-100 text-gray-700';
      case 'sending': return 'bg-brand-50 text-brand-700';
      case 'paused': return 'bg-amber-50 text-amber-700';
      case 'bounced': case 'cancelled': return 'bg-red-50 text-red-700';
      case 'unsubscribed': return 'bg-amber-50 text-amber-700';
      default: return 'bg-gray-100 text-gray-700';
    }
  };

  const statCards = stats ? [
    { label: 'Campaigns', value: stats.total_campaigns ?? 0, icon: <Mail size={18} />, color: 'text-brand-600' },
    { label: 'Contacts', value: stats.total_contacts ?? 0, icon: <Users size={18} />, color: 'text-emerald-600' },
    { label: 'Emails Sent', value: stats.total_sent ?? 0, icon: <Send size={18} />, color: 'text-sky-600' },
    { label: 'Open Rate', value: toPercent(stats.avg_open_rate), icon: <Eye size={18} />, color: 'text-gray-600' },
    { label: 'Click Rate', value: toPercent(stats.avg_click_rate), icon: <MousePointer size={18} />, color: 'text-gray-600' },
    { label: 'Bounce Rate', value: toPercent(stats.avg_bounce_rate), icon: <AlertTriangle size={18} />, color: 'text-red-600' },
  ] : [];

  const createLabel = activeTab === 'campaigns' ? 'Campaign' : activeTab === 'contacts' ? 'Contact' : activeTab === 'lists' ? 'List' : 'Template';

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold text-gray-900">
            <Mail className="text-gray-500" size={20} />
            Email Marketing
          </h1>
          <p className="mt-1 text-sm text-gray-600">
            Create campaigns, manage contacts, and track email performance
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button type="button" onClick={refreshAll} className={BTN_SECONDARY}>
            <RefreshCw size={16} /> Refresh
          </button>
          <button
            type="button"
            onClick={() => { setCreateType(activeTab === 'campaigns' ? 'campaign' : activeTab === 'contacts' ? 'contact' : activeTab === 'lists' ? 'list' : 'template'); setShowCreateModal(true); }}
            className={BTN_PRIMARY}
          >
            <Plus size={16} /> New {createLabel}
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {/* Stats Cards */}
      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
          {statCards.map((card, i) => (
            <div key={i} className={`${CARD} p-4`}>
              <div className="mb-2 flex items-center justify-between">
                <span className={card.color}>{card.icon}</span>
              </div>
              <p className="text-2xl font-semibold text-gray-900">{card.value}</p>
              <p className="mt-1 text-xs font-medium uppercase tracking-wide text-gray-500">{card.label}</p>
            </div>
          ))}
        </div>
      )}

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="Email marketing sections">
          {[
            { id: 'campaigns', label: 'Campaigns', icon: <Mail size={16} /> },
            { id: 'contacts', label: 'Contacts', icon: <Users size={16} /> },
            { id: 'lists', label: 'Lists', icon: <Inbox size={16} /> },
            { id: 'templates', label: 'Templates', icon: <FileText size={16} /> },
          ].map((tab) => (
            <button
              key={tab.id}
              type="button"
              aria-current={activeTab === tab.id ? 'page' : undefined}
              onClick={() => setActiveTab(tab.id as any)}
              className={`flex items-center gap-2 border-b-2 px-1 pb-3 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 ${
                activeTab === tab.id
                  ? 'border-brand-600 text-brand-700'
                  : 'border-transparent text-gray-600 hover:border-gray-300 hover:text-gray-900'
              }`}
            >
              {tab.icon} {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {loading && (
        <div className={`${CARD} px-5 py-8 text-center text-sm text-gray-500`}>Loading…</div>
      )}

      {/* Campaigns Tab */}
      {!loading && activeTab === 'campaigns' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Campaigns ({campaigns.length})</h2>
          </div>
          {campaigns.length === 0 ? (
            <div className="px-5 py-12 text-center">
              <Mail className="mx-auto mb-3 text-gray-400" size={36} />
              <p className="text-sm text-gray-600">No campaigns yet. Create your first email campaign.</p>
            </div>
          ) : (
            <div>
              {campaigns.map((c) => (
                <div key={c.id} className="border-b border-gray-100 px-5 py-4 hover:bg-gray-50">
                  <div className="flex items-start justify-between gap-4">
                    <div className="flex-1">
                      <div className="flex items-center gap-3">
                        <h3 className="text-sm font-semibold text-gray-900">{c.name}</h3>
                        <span className={`${BADGE} ${getStatusColor(c.status)}`}>{c.status || 'unknown'}</span>
                      </div>
                      <p className="mt-1 text-sm text-gray-600">{c.subject}</p>
                      <div className="mt-2 flex flex-wrap items-center gap-4 text-xs text-gray-500">
                        <span>From: {c.from_name} &lt;{c.from_email}&gt;</span>
                        <span>{c.total_recipients ?? 0} recipients</span>
                        <span>{c.sent_count ?? 0} sent</span>
                        <span>{c.open_count ?? 0} opened ({(c.total_recipients ?? 0) > 0 ? (((c.open_count ?? 0) / (c.total_recipients ?? 1)) * 100).toFixed(1) : 0}%)</span>
                        <span>{c.click_count ?? 0} clicked</span>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      {(c.status === 'draft' || c.status === 'paused') && (
                        <button
                          type="button"
                          onClick={() => handleSendCampaign(c.id)}
                          className="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                        >
                          <Send size={12} /> Send
                        </button>
                      )}
                      {c.status === 'sending' && (
                        <button
                          type="button"
                          onClick={() => handlePauseCampaign(c.id)}
                          className="inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                        >
                          <Pause size={12} /> Pause
                        </button>
                      )}
                      <button
                        type="button"
                        aria-label={`Delete campaign ${c.name}`}
                        onClick={() => handleDelete('campaigns', c.id)}
                        className="inline-flex items-center gap-1 rounded-md border border-red-300 bg-white px-3 py-1.5 text-xs font-medium text-red-700 hover:bg-red-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
                      >
                        <Trash2 size={12} />
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Contacts Tab */}
      {!loading && activeTab === 'contacts' && (
        <div className={CARD}>
          <div className={`${CARD_HEADER} flex items-center justify-between gap-4`}>
            <h2 className="text-sm font-semibold text-gray-900">Contacts ({contacts.length})</h2>
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" size={16} />
              <input
                type="text"
                aria-label="Search contacts"
                placeholder="Search contacts..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="w-64 rounded-md border border-gray-300 bg-white py-2 pl-9 pr-3 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus:outline-none"
              />
            </div>
          </div>
          {contacts.length === 0 ? (
            <div className="px-5 py-12 text-center">
              <Users className="mx-auto mb-3 text-gray-400" size={36} />
              <p className="text-sm text-gray-600">No contacts yet. Add your first subscriber.</p>
            </div>
          ) : (
            <div>
              {contacts.map((c) => (
                <div key={c.id} className="flex items-center justify-between gap-4 border-b border-gray-100 px-5 py-4 hover:bg-gray-50">
                  <div>
                    <p className="text-sm font-semibold text-gray-900">{c.email}</p>
                    <p className="mt-0.5 text-sm text-gray-600">
                      {[c.first_name, c.last_name].filter(Boolean).join(' ') || 'No name'} • Source: {c.source || '—'}
                    </p>
                    {Array.isArray(c.tags) && c.tags.length > 0 && (
                      <div className="mt-1 flex flex-wrap gap-1">
                        {c.tags.map((tag, i) => (
                          <span key={i} className={`${BADGE} bg-brand-50 text-brand-700`}>{tag}</span>
                        ))}
                      </div>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <span className={`${BADGE} ${getStatusColor(c.status)}`}>{c.status || 'unknown'}</span>
                    <button
                      type="button"
                      aria-label={`Delete contact ${c.email}`}
                      onClick={() => handleDelete('contacts', c.id)}
                      className={ICON_DANGER}
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Lists Tab */}
      {!loading && activeTab === 'lists' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Mailing Lists ({lists.length})</h2>
          </div>
          {lists.length === 0 ? (
            <div className="px-5 py-12 text-center">
              <Inbox className="mx-auto mb-3 text-gray-400" size={36} />
              <p className="text-sm text-gray-600">No mailing lists yet. Create your first list.</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-4 p-5 md:grid-cols-2 lg:grid-cols-3">
              {lists.map((l) => (
                <div key={l.id} className="rounded-md border border-gray-200 p-4 hover:border-gray-300">
                  <div className="mb-2 flex items-start justify-between gap-2">
                    <h3 className="text-sm font-semibold text-gray-900">{l.name}</h3>
                    <button
                      type="button"
                      aria-label={`Delete list ${l.name}`}
                      onClick={() => handleDelete('lists', l.id)}
                      className={ICON_DANGER}
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                  <p className="mb-3 text-sm text-gray-600">{l.description || 'No description'}</p>
                  <div className="flex items-center justify-between text-xs text-gray-500">
                    <span className="flex items-center gap-1"><Users size={12} /> {l.contact_count ?? 0} contacts</span>
                    {l.double_opt_in && <span className={`${BADGE} bg-emerald-50 text-emerald-700`}>Double Opt-in</span>}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Templates Tab */}
      {!loading && activeTab === 'templates' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Email Templates ({templates.length})</h2>
          </div>
          {templates.length === 0 ? (
            <div className="px-5 py-12 text-center">
              <FileText className="mx-auto mb-3 text-gray-400" size={36} />
              <p className="text-sm text-gray-600">No templates yet. Create your first email template.</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-4 p-5 md:grid-cols-2 lg:grid-cols-3">
              {templates.map((t) => (
                <div key={t.id} className="rounded-md border border-gray-200 p-4 hover:border-gray-300">
                  <div className="mb-2 flex items-start justify-between gap-2">
                    <h3 className="text-sm font-semibold text-gray-900">{t.name}</h3>
                    <button
                      type="button"
                      aria-label={`Delete template ${t.name}`}
                      onClick={() => handleDelete('templates', t.id)}
                      className={ICON_DANGER}
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                  <p className="mb-2 text-sm text-gray-600">{t.subject}</p>
                  {t.category && (
                    <span className={`${BADGE} bg-gray-100 text-gray-700`}>{t.category}</span>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Create Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40 p-4">
          <div className="max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg">
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <h2 className="text-sm font-semibold text-gray-900">
                Create {createType.charAt(0).toUpperCase() + createType.slice(1)}
              </h2>
              <button
                type="button"
                aria-label="Close dialog"
                onClick={() => setShowCreateModal(false)}
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X size={18} />
              </button>
            </div>
            <div className="space-y-4 p-5">
              {createType === 'campaign' && (
                <>
                  <div>
                    <label htmlFor="campaign-name" className={LABEL}>Campaign Name</label>
                    <input id="campaign-name" type="text" value={campaignForm.name} onChange={(e) => setCampaignForm({...campaignForm, name: e.target.value})}
                      className={INPUT} placeholder="Summer Sale 2024" />
                  </div>
                  <div>
                    <label htmlFor="campaign-subject" className={LABEL}>Subject Line</label>
                    <input id="campaign-subject" type="text" value={campaignForm.subject} onChange={(e) => setCampaignForm({...campaignForm, subject: e.target.value})}
                      className={INPUT} placeholder="Don't miss our summer deals!" />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label htmlFor="campaign-from-name" className={LABEL}>From Name</label>
                      <input id="campaign-from-name" type="text" value={campaignForm.from_name} onChange={(e) => setCampaignForm({...campaignForm, from_name: e.target.value})}
                        className={INPUT} />
                    </div>
                    <div>
                      <label htmlFor="campaign-from-email" className={LABEL}>From Email</label>
                      <input id="campaign-from-email" type="email" value={campaignForm.from_email} onChange={(e) => setCampaignForm({...campaignForm, from_email: e.target.value})}
                        className={INPUT} placeholder="hello@example.com" />
                    </div>
                  </div>
                  <div>
                    <label htmlFor="campaign-html" className={LABEL}>HTML Content</label>
                    <textarea id="campaign-html" value={campaignForm.html_content} onChange={(e) => setCampaignForm({...campaignForm, html_content: e.target.value})}
                      className={`${INPUT} h-32`} placeholder="<h1>Hello!</h1><p>Your content here...</p>" />
                  </div>
                </>
              )}
              {createType === 'contact' && (
                <>
                  <div>
                    <label htmlFor="contact-email" className={LABEL}>Email</label>
                    <input id="contact-email" type="email" value={contactForm.email} onChange={(e) => setContactForm({...contactForm, email: e.target.value})}
                      className={INPUT} placeholder="subscriber@example.com" />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label htmlFor="contact-first-name" className={LABEL}>First Name</label>
                      <input id="contact-first-name" type="text" value={contactForm.first_name} onChange={(e) => setContactForm({...contactForm, first_name: e.target.value})}
                        className={INPUT} />
                    </div>
                    <div>
                      <label htmlFor="contact-last-name" className={LABEL}>Last Name</label>
                      <input id="contact-last-name" type="text" value={contactForm.last_name} onChange={(e) => setContactForm({...contactForm, last_name: e.target.value})}
                        className={INPUT} />
                    </div>
                  </div>
                </>
              )}
              {createType === 'list' && (
                <>
                  <div>
                    <label htmlFor="list-name" className={LABEL}>List Name</label>
                    <input id="list-name" type="text" value={listForm.name} onChange={(e) => setListForm({...listForm, name: e.target.value})}
                      className={INPUT} placeholder="Newsletter Subscribers" />
                  </div>
                  <div>
                    <label htmlFor="list-description" className={LABEL}>Description</label>
                    <textarea id="list-description" value={listForm.description} onChange={(e) => setListForm({...listForm, description: e.target.value})}
                      className={`${INPUT} h-20`} />
                  </div>
                  <label className="flex items-center gap-2">
                    <input type="checkbox" checked={listForm.double_opt_in} onChange={(e) => setListForm({...listForm, double_opt_in: e.target.checked})}
                      className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500" />
                    <span className="text-sm text-gray-700">Enable Double Opt-in</span>
                  </label>
                </>
              )}
              {createType === 'template' && (
                <>
                  <div>
                    <label htmlFor="template-name" className={LABEL}>Template Name</label>
                    <input id="template-name" type="text" value={templateForm.name} onChange={(e) => setTemplateForm({...templateForm, name: e.target.value})}
                      className={INPUT} placeholder="Welcome Email" />
                  </div>
                  <div>
                    <label htmlFor="template-subject" className={LABEL}>Subject</label>
                    <input id="template-subject" type="text" value={templateForm.subject} onChange={(e) => setTemplateForm({...templateForm, subject: e.target.value})}
                      className={INPUT} />
                  </div>
                  <div>
                    <label htmlFor="template-category" className={LABEL}>Category</label>
                    <input id="template-category" type="text" value={templateForm.category} onChange={(e) => setTemplateForm({...templateForm, category: e.target.value})}
                      className={INPUT} placeholder="welcome, promo, newsletter" />
                  </div>
                  <div>
                    <label htmlFor="template-html" className={LABEL}>HTML Content</label>
                    <textarea id="template-html" value={templateForm.html_content} onChange={(e) => setTemplateForm({...templateForm, html_content: e.target.value})}
                      className={`${INPUT} h-32`} />
                  </div>
                </>
              )}
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              <button type="button" onClick={() => setShowCreateModal(false)} className={BTN_SECONDARY}>Cancel</button>
              <button type="button" onClick={handleCreate} className={BTN_PRIMARY}>Create</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
