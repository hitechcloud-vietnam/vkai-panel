'use client';

import { useState, useEffect } from 'react';
import { 
  Mail, Plus, Send, Pause, Trash2, RefreshCw, Search,
  Users, FileText, BarChart3, Eye, MousePointer, AlertTriangle,
  X, Clock, CheckCircle, XCircle, TrendingUp, Inbox
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

export default function EmailMarketingPage() {
  const [activeTab, setActiveTab] = useState<'campaigns' | 'contacts' | 'lists' | 'templates'>('campaigns');
  const [stats, setStats] = useState<EmailStats | null>(null);
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [lists, setLists] = useState<EmailList[]>([]);
  const [templates, setTemplates] = useState<EmailTemplate[]>([]);
  const [loading, setLoading] = useState(true);
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

  const getToken = () => localStorage.getItem('token') || '';

  const fetchStats = async () => {
    try {
      const res = await fetch('/api/v1/email-marketing/stats', {
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) {
        const data = await res.json();
        setStats(data.stats);
      }
    } catch (e) { console.error(e); }
  };

  const fetchCampaigns = async () => {
    try {
      const res = await fetch('/api/v1/email-marketing/campaigns?limit=100&offset=0', {
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) {
        const data = await res.json();
        setCampaigns(data.campaigns || []);
      }
    } catch (e) { console.error(e); }
  };

  const fetchContacts = async () => {
    try {
      const res = await fetch(`/api/v1/email-marketing/contacts?limit=100&offset=0&search=${searchTerm}`, {
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) {
        const data = await res.json();
        setContacts(data.contacts || []);
      }
    } catch (e) { console.error(e); }
  };

  const fetchLists = async () => {
    try {
      const res = await fetch('/api/v1/email-marketing/lists', {
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) {
        const data = await res.json();
        setLists(data.lists || []);
      }
    } catch (e) { console.error(e); }
  };

  const fetchTemplates = async () => {
    try {
      const res = await fetch('/api/v1/email-marketing/templates', {
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) {
        const data = await res.json();
        setTemplates(data.templates || []);
      }
    } catch (e) { console.error(e); }
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
      }
    } catch (e) { console.error(e); }
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
    } catch (e) { console.error(e); }
  };

  const handleSendCampaign = async (id: string) => {
    try {
      const res = await fetch(`/api/v1/email-marketing/campaigns/${id}/send`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) fetchCampaigns();
    } catch (e) { console.error(e); }
  };

  const handlePauseCampaign = async (id: string) => {
    try {
      const res = await fetch(`/api/v1/email-marketing/campaigns/${id}/pause`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${getToken()}` }
      });
      if (res.ok) fetchCampaigns();
    } catch (e) { console.error(e); }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'active': case 'sent': case 'delivered': return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400';
      case 'draft': case 'queued': return 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300';
      case 'sending': return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400';
      case 'paused': return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400';
      case 'bounced': case 'cancelled': return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400';
      case 'unsubscribed': return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400';
      default: return 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300';
    }
  };

  const statCards = stats ? [
    { label: 'Campaigns', value: stats.total_campaigns, icon: <Mail size={20} />, color: 'text-blue-500' },
    { label: 'Contacts', value: stats.total_contacts, icon: <Users size={20} />, color: 'text-green-500' },
    { label: 'Emails Sent', value: stats.total_sent, icon: <Send size={20} />, color: 'text-purple-500' },
    { label: 'Open Rate', value: `${stats.avg_open_rate.toFixed(1)}%`, icon: <Eye size={20} />, color: 'text-orange-500' },
    { label: 'Click Rate', value: `${stats.avg_click_rate.toFixed(1)}%`, icon: <MousePointer size={20} />, color: 'text-pink-500' },
    { label: 'Bounce Rate', value: `${stats.avg_bounce_rate.toFixed(1)}%`, icon: <AlertTriangle size={20} />, color: 'text-red-500' },
  ] : [];

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
            <Mail className="text-blue-500" size={28} />
            Email Marketing
          </h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">
            Create campaigns, manage contacts, and track email performance
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button onClick={() => { fetchStats(); fetchCampaigns(); fetchContacts(); fetchLists(); fetchTemplates(); }}
            className="px-4 py-2 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600 flex items-center gap-2">
            <RefreshCw size={16} /> Refresh
          </button>
          <button onClick={() => { setCreateType(activeTab === 'campaigns' ? 'campaign' : activeTab === 'contacts' ? 'contact' : activeTab === 'lists' ? 'list' : 'template'); setShowCreateModal(true); }}
            className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 flex items-center gap-2">
            <Plus size={16} /> New {activeTab === 'campaigns' ? 'Campaign' : activeTab === 'contacts' ? 'Contact' : activeTab === 'lists' ? 'List' : 'Template'}
          </button>
        </div>
      </div>

      {/* Stats Cards */}
      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
          {statCards.map((card, i) => (
            <div key={i} className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 p-4">
              <div className="flex items-center justify-between mb-2">
                <span className={card.color}>{card.icon}</span>
              </div>
              <p className="text-2xl font-bold text-gray-900 dark:text-white">{card.value}</p>
              <p className="text-xs text-gray-500 dark:text-gray-400">{card.label}</p>
            </div>
          ))}
        </div>
      )}

      {/* Tabs */}
      <div className="flex space-x-1 bg-gray-100 dark:bg-gray-800 p-1 rounded-lg w-fit">
        {[
          { id: 'campaigns', label: 'Campaigns', icon: <Mail size={16} /> },
          { id: 'contacts', label: 'Contacts', icon: <Users size={16} /> },
          { id: 'lists', label: 'Lists', icon: <Inbox size={16} /> },
          { id: 'templates', label: 'Templates', icon: <FileText size={16} /> },
        ].map((tab) => (
          <button key={tab.id} onClick={() => setActiveTab(tab.id as any)}
            className={`px-4 py-2 rounded-md text-sm font-medium flex items-center gap-2 transition-colors ${
              activeTab === tab.id
                ? 'bg-white dark:bg-gray-700 text-blue-600 dark:text-blue-400 shadow-sm'
                : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
            }`}>
            {tab.icon} {tab.label}
          </button>
        ))}
      </div>

      {/* Campaigns Tab */}
      {activeTab === 'campaigns' && (
        <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Campaigns ({campaigns.length})</h3>
          </div>
          {campaigns.length === 0 ? (
            <div className="p-12 text-center text-gray-500 dark:text-gray-400">
              <Mail className="mx-auto mb-3 text-gray-300 dark:text-gray-600" size={40} />
              <p>No campaigns yet. Create your first email campaign!</p>
            </div>
          ) : (
            <div className="divide-y divide-gray-100 dark:divide-gray-700">
              {campaigns.map((c) => (
                <div key={c.id} className="p-4 hover:bg-gray-50 dark:hover:bg-gray-700/50">
                  <div className="flex items-center justify-between">
                    <div className="flex-1">
                      <div className="flex items-center gap-3">
                        <h4 className="font-medium text-gray-900 dark:text-white">{c.name}</h4>
                        <span className={`px-2 py-0.5 text-xs rounded-full ${getStatusColor(c.status)}`}>{c.status}</span>
                      </div>
                      <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">{c.subject}</p>
                      <div className="flex items-center gap-4 mt-2 text-xs text-gray-500 dark:text-gray-400">
                        <span>From: {c.from_name} &lt;{c.from_email}&gt;</span>
                        <span>{c.total_recipients} recipients</span>
                        <span>{c.sent_count} sent</span>
                        <span>{c.open_count} opened ({c.total_recipients > 0 ? ((c.open_count / c.total_recipients) * 100).toFixed(1) : 0}%)</span>
                        <span>{c.click_count} clicked</span>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      {(c.status === 'draft' || c.status === 'paused') && (
                        <button onClick={() => handleSendCampaign(c.id)}
                          className="px-3 py-1.5 text-xs bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400 rounded-lg hover:bg-green-200 flex items-center gap-1">
                          <Send size={12} /> Send
                        </button>
                      )}
                      {c.status === 'sending' && (
                        <button onClick={() => handlePauseCampaign(c.id)}
                          className="px-3 py-1.5 text-xs bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400 rounded-lg hover:bg-yellow-200 flex items-center gap-1">
                          <Pause size={12} /> Pause
                        </button>
                      )}
                      <button onClick={() => handleDelete('campaigns', c.id)}
                        className="px-3 py-1.5 text-xs bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400 rounded-lg hover:bg-red-200 flex items-center gap-1">
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
      {activeTab === 'contacts' && (
        <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Contacts ({contacts.length})</h3>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400" size={16} />
              <input type="text" placeholder="Search contacts..." value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="pl-10 pr-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm" />
            </div>
          </div>
          {contacts.length === 0 ? (
            <div className="p-12 text-center text-gray-500 dark:text-gray-400">
              <Users className="mx-auto mb-3 text-gray-300 dark:text-gray-600" size={40} />
              <p>No contacts yet. Add your first subscriber!</p>
            </div>
          ) : (
            <div className="divide-y divide-gray-100 dark:divide-gray-700">
              {contacts.map((c) => (
                <div key={c.id} className="p-4 hover:bg-gray-50 dark:hover:bg-gray-700/50 flex items-center justify-between">
                  <div>
                    <p className="font-medium text-gray-900 dark:text-white">{c.email}</p>
                    <p className="text-sm text-gray-500 dark:text-gray-400">
                      {[c.first_name, c.last_name].filter(Boolean).join(' ') || 'No name'} • Source: {c.source}
                    </p>
                    {c.tags && c.tags.length > 0 && (
                      <div className="flex gap-1 mt-1">
                        {c.tags.map((tag, i) => (
                          <span key={i} className="px-2 py-0.5 text-xs bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400 rounded-full">{tag}</span>
                        ))}
                      </div>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <span className={`px-2 py-0.5 text-xs rounded-full ${getStatusColor(c.status)}`}>{c.status}</span>
                    <button onClick={() => handleDelete('contacts', c.id)}
                      className="p-1.5 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 rounded">
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
      {activeTab === 'lists' && (
        <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Mailing Lists ({lists.length})</h3>
          </div>
          {lists.length === 0 ? (
            <div className="p-12 text-center text-gray-500 dark:text-gray-400">
              <Inbox className="mx-auto mb-3 text-gray-300 dark:text-gray-600" size={40} />
              <p>No mailing lists yet. Create your first list!</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 p-4">
              {lists.map((l) => (
                <div key={l.id} className="border border-gray-200 dark:border-gray-700 rounded-lg p-4 hover:border-blue-300 dark:hover:border-blue-700">
                  <div className="flex items-center justify-between mb-2">
                    <h4 className="font-medium text-gray-900 dark:text-white">{l.name}</h4>
                    <button onClick={() => handleDelete('lists', l.id)}
                      className="p-1 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 rounded">
                      <Trash2 size={14} />
                    </button>
                  </div>
                  <p className="text-sm text-gray-500 dark:text-gray-400 mb-3">{l.description || 'No description'}</p>
                  <div className="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
                    <span className="flex items-center gap-1"><Users size={12} /> {l.contact_count} contacts</span>
                    {l.double_opt_in && <span className="text-green-600 dark:text-green-400">Double Opt-in</span>}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Templates Tab */}
      {activeTab === 'templates' && (
        <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700">
          <div className="p-4 border-b border-gray-200 dark:border-gray-700">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Email Templates ({templates.length})</h3>
          </div>
          {templates.length === 0 ? (
            <div className="p-12 text-center text-gray-500 dark:text-gray-400">
              <FileText className="mx-auto mb-3 text-gray-300 dark:text-gray-600" size={40} />
              <p>No templates yet. Create your first email template!</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 p-4">
              {templates.map((t) => (
                <div key={t.id} className="border border-gray-200 dark:border-gray-700 rounded-lg p-4 hover:border-blue-300 dark:hover:border-blue-700">
                  <div className="flex items-center justify-between mb-2">
                    <h4 className="font-medium text-gray-900 dark:text-white">{t.name}</h4>
                    <button onClick={() => handleDelete('templates', t.id)}
                      className="p-1 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 rounded">
                      <Trash2 size={14} />
                    </button>
                  </div>
                  <p className="text-sm text-gray-500 dark:text-gray-400 mb-2">{t.subject}</p>
                  {t.category && (
                    <span className="px-2 py-0.5 text-xs bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400 rounded-full">{t.category}</span>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Create Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-lg max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-xl font-bold text-gray-900 dark:text-white">
                Create {createType.charAt(0).toUpperCase() + createType.slice(1)}
              </h2>
              <button onClick={() => setShowCreateModal(false)} className="text-gray-400 hover:text-gray-600">
                <X size={20} />
              </button>
            </div>
            <div className="space-y-4">
              {createType === 'campaign' && (
                <>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Campaign Name</label>
                    <input type="text" value={campaignForm.name} onChange={(e) => setCampaignForm({...campaignForm, name: e.target.value})}
                      className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" placeholder="Summer Sale 2024" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Subject Line</label>
                    <input type="text" value={campaignForm.subject} onChange={(e) => setCampaignForm({...campaignForm, subject: e.target.value})}
                      className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" placeholder="Don't miss our summer deals!" />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">From Name</label>
                      <input type="text" value={campaignForm.from_name} onChange={(e) => setCampaignForm({...campaignForm, from_name: e.target.value})}
                        className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">From Email</label>
                      <input type="email" value={campaignForm.from_email} onChange={(e) => setCampaignForm({...campaignForm, from_email: e.target.value})}
                        className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" placeholder="hello@example.com" />
                    </div>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">HTML Content</label>
                    <textarea value={campaignForm.html_content} onChange={(e) => setCampaignForm({...campaignForm, html_content: e.target.value})}
                      className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white h-32" placeholder="<h1>Hello!</h1><p>Your content here...</p>" />
                  </div>
                </>
              )}
              {createType === 'contact' && (
                <>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Email</label>
                    <input type="email" value={contactForm.email} onChange={(e) => setContactForm({...contactForm, email: e.target.value})}
                      className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" placeholder="subscriber@example.com" />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">First Name</label>
                      <input type="text" value={contactForm.first_name} onChange={(e) => setContactForm({...contactForm, first_name: e.target.value})}
                        className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Last Name</label>
                      <input type="text" value={contactForm.last_name} onChange={(e) => setContactForm({...contactForm, last_name: e.target.value})}
                        className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
                    </div>
                  </div>
                </>
              )}
              {createType === 'list' && (
                <>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">List Name</label>
                    <input type="text" value={listForm.name} onChange={(e) => setListForm({...listForm, name: e.target.value})}
                      className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" placeholder="Newsletter Subscribers" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Description</label>
                    <textarea value={listForm.description} onChange={(e) => setListForm({...listForm, description: e.target.value})}
                      className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white h-20" />
                  </div>
                  <label className="flex items-center gap-2">
                    <input type="checkbox" checked={listForm.double_opt_in} onChange={(e) => setListForm({...listForm, double_opt_in: e.target.checked})}
                      className="rounded border-gray-300" />
                    <span className="text-sm text-gray-700 dark:text-gray-300">Enable Double Opt-in</span>
                  </label>
                </>
              )}
              {createType === 'template' && (
                <>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Template Name</label>
                    <input type="text" value={templateForm.name} onChange={(e) => setTemplateForm({...templateForm, name: e.target.value})}
                      className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" placeholder="Welcome Email" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Subject</label>
                    <input type="text" value={templateForm.subject} onChange={(e) => setTemplateForm({...templateForm, subject: e.target.value})}
                      className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Category</label>
                    <input type="text" value={templateForm.category} onChange={(e) => setTemplateForm({...templateForm, category: e.target.value})}
                      className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white" placeholder="welcome, promo, newsletter" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">HTML Content</label>
                    <textarea value={templateForm.html_content} onChange={(e) => setTemplateForm({...templateForm, html_content: e.target.value})}
                      className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white h-32" />
                  </div>
                </>
              )}
            </div>
            <div className="flex justify-end gap-3 mt-6">
              <button onClick={() => setShowCreateModal(false)}
                className="px-4 py-2 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600">Cancel</button>
              <button onClick={handleCreate}
                className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600">Create</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
