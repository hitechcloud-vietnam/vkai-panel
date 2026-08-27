'use client';

import { useState, useEffect } from 'react';
import {
  Globe, Plus, Settings, Trash2, RefreshCw, ExternalLink,
  Package, Palette, Shield, CheckCircle, XCircle, AlertCircle,
  Search
} from 'lucide-react';

interface WordPressSite {
  id: string;
  tenant_id: string;
  server_id: string;
  website_id: string;
  domain: string;
  name: string;
  admin_user: string;
  admin_email: string;
  version: string;
  status: string;
  auto_update: boolean;
  is_active: boolean;
  db_name: string;
  db_user: string;
  db_host: string;
  db_prefix: string;
  created_at: string;
  updated_at: string;
}

interface WordPressPlugin {
  id: string;
  site_id: string;
  name: string;
  slug: string;
  version: string;
  status: string;
  description: string;
  author: string;
  created_at: string;
  updated_at: string;
}

interface WordPressTheme {
  id: string;
  site_id: string;
  name: string;
  slug: string;
  version: string;
  active: boolean;
  description: string;
  author: string;
  created_at: string;
  updated_at: string;
}

const CARD = 'bg-white border border-gray-200 rounded-lg shadow-sm';
const CARD_HEADER = 'px-5 py-4 border-b border-gray-200';
const INPUT =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus:outline-none';
const LABEL = 'mb-1.5 block text-sm font-medium text-gray-700';
const BADGE = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium';
const BTN_PRIMARY =
  'inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1';
const BTN_SECONDARY =
  'inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1';
const BTN_SMALL =
  'inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500';
const BTN_SMALL_DANGER =
  'inline-flex items-center gap-1 rounded-md border border-red-300 bg-white px-2.5 py-1.5 text-xs font-medium text-red-700 hover:bg-red-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500';

function formatDateTime(value?: string): string {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString();
}

function getToken(): string {
  if (typeof window === 'undefined') return '';
  try {
    return localStorage.getItem('token') || '';
  } catch {
    return '';
  }
}

export default function WordPressToolkitPage() {
  const [sites, setSites] = useState<WordPressSite[]>([]);
  const [selectedSite, setSelectedSite] = useState<WordPressSite | null>(null);
  const [plugins, setPlugins] = useState<WordPressPlugin[]>([]);
  const [themes, setThemes] = useState<WordPressTheme[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [activeTab, setActiveTab] = useState<'sites' | 'plugins' | 'themes' | 'security'>('sites');
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [total, setTotal] = useState(0);

  // Create form state
  const [createForm, setCreateForm] = useState({
    server_id: '',
    domain: '',
    name: '',
    admin_user: 'admin',
    admin_email: 'admin@example.com',
    admin_password: '',
    db_name: '',
    db_user: '',
    db_password: '',
    db_host: 'localhost',
    db_prefix: 'wp_',
  });

  const fetchSites = async () => {
    setLoading(true);
    setError('');
    try {
      const token = getToken();
      const res = await fetch('/api/v1/wordpress?limit=100&offset=0', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setSites(Array.isArray(data?.sites) ? data.sites : []);
        setTotal(typeof data?.total === 'number' ? data.total : 0);
      } else {
        setError('Unable to load WordPress sites.');
      }
    } catch (error) {
      console.error('Failed to fetch WordPress sites:', error);
      setError('Unable to load WordPress sites. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  const fetchPlugins = async (siteId: string) => {
    try {
      const token = getToken();
      const res = await fetch(`/api/v1/wordpress/${siteId}/plugins`, {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setPlugins(Array.isArray(data?.plugins) ? data.plugins : []);
      }
    } catch (error) {
      console.error('Failed to fetch plugins:', error);
      setError('Unable to load plugins. Please try again.');
    }
  };

  const fetchThemes = async (siteId: string) => {
    try {
      const token = getToken();
      const res = await fetch(`/api/v1/wordpress/${siteId}/themes`, {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setThemes(Array.isArray(data?.themes) ? data.themes : []);
      }
    } catch (error) {
      console.error('Failed to fetch themes:', error);
      setError('Unable to load themes. Please try again.');
    }
  };

  useEffect(() => {
    fetchSites();
  }, []);

  useEffect(() => {
    if (selectedSite) {
      fetchPlugins(selectedSite.id);
      fetchThemes(selectedSite.id);
    }
  }, [selectedSite]);

  const handleCreateSite = async () => {
    try {
      const token = getToken();
      const res = await fetch('/api/v1/wordpress', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(createForm)
      });
      if (res.ok) {
        setShowCreateModal(false);
        fetchSites();
        setCreateForm({
          server_id: '', domain: '', name: '', admin_user: 'admin',
          admin_email: 'admin@example.com', admin_password: '',
          db_name: '', db_user: '', db_password: '', db_host: 'localhost', db_prefix: 'wp_',
        });
      } else {
        setError('The server rejected the request. Please review the form and try again.');
      }
    } catch (error) {
      console.error('Failed to create WordPress site:', error);
      setError('Unable to create the site. Please try again.');
    }
  };

  const handleDeleteSite = async (siteId: string) => {
    if (!confirm('Are you sure you want to delete this WordPress site? This action cannot be undone.')) return;
    try {
      const token = getToken();
      const res = await fetch(`/api/v1/wordpress/${siteId}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        if (selectedSite?.id === siteId) setSelectedSite(null);
        fetchSites();
      }
    } catch (error) {
      console.error('Failed to delete site:', error);
      setError('Unable to delete the site. Please try again.');
    }
  };

  const handleToggleAutoUpdate = async (site: WordPressSite) => {
    try {
      const token = getToken();
      const res = await fetch(`/api/v1/wordpress/${site.id}`, {
        method: 'PUT',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ auto_update: !site.auto_update })
      });
      if (res.ok) fetchSites();
    } catch (error) {
      console.error('Failed to toggle auto-update:', error);
      setError('Unable to change the auto-update setting. Please try again.');
    }
  };

  const handleTogglePlugin = async (siteId: string, plugin: WordPressPlugin) => {
    try {
      const token = getToken();
      const newStatus = plugin.status === 'active' ? 'inactive' : 'active';
      const res = await fetch(`/api/v1/wordpress/${siteId}/plugins/${plugin.id}`, {
        method: 'PUT',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ status: newStatus })
      });
      if (res.ok && selectedSite) fetchPlugins(selectedSite.id);
    } catch (error) {
      console.error('Failed to toggle plugin:', error);
      setError('Unable to change the plugin state. Please try again.');
    }
  };

  const handleDeletePlugin = async (siteId: string, pluginId: string) => {
    if (!confirm('Are you sure you want to delete this plugin?')) return;
    try {
      const token = getToken();
      const res = await fetch(`/api/v1/wordpress/${siteId}/plugins/${pluginId}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok && selectedSite) fetchPlugins(selectedSite.id);
    } catch (error) {
      console.error('Failed to delete plugin:', error);
      setError('Unable to delete the plugin. Please try again.');
    }
  };

  const handleActivateTheme = async (siteId: string, themeId: string) => {
    try {
      const token = getToken();
      const res = await fetch(`/api/v1/wordpress/${siteId}/themes/${themeId}`, {
        method: 'PUT',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ active: true })
      });
      if (res.ok && selectedSite) fetchThemes(selectedSite.id);
    } catch (error) {
      console.error('Failed to activate theme:', error);
      setError('Unable to activate the theme. Please try again.');
    }
  };

  const handleDeleteTheme = async (siteId: string, themeId: string) => {
    if (!confirm('Are you sure you want to delete this theme?')) return;
    try {
      const token = getToken();
      const res = await fetch(`/api/v1/wordpress/${siteId}/themes/${themeId}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok && selectedSite) fetchThemes(selectedSite.id);
    } catch (error) {
      console.error('Failed to delete theme:', error);
      setError('Unable to delete the theme. Please try again.');
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'active': return 'bg-emerald-50 text-emerald-700';
      case 'inactive': return 'bg-gray-100 text-gray-700';
      case 'error': return 'bg-red-50 text-red-700';
      default: return 'bg-amber-50 text-amber-700';
    }
  };

  const filteredSites = sites.filter(s =>
    (s?.domain || '').toLowerCase().includes(searchTerm.toLowerCase()) ||
    (s?.name || '').toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold text-gray-900">
            <Globe className="text-gray-500" size={20} />
            WordPress Toolkit Pro
          </h1>
          <p className="mt-1 text-sm text-gray-600">
            Manage WordPress installations, plugins, themes, and security
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button type="button" onClick={fetchSites} className={BTN_SECONDARY}>
            <RefreshCw size={16} />
            Refresh
          </button>
          <button type="button" onClick={() => setShowCreateModal(true)} className={BTN_PRIMARY}>
            <Plus size={16} />
            New Site
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="WordPress sections">
          {[
            { id: 'sites', label: 'Sites', icon: <Globe size={16} /> },
            { id: 'plugins', label: 'Plugins', icon: <Package size={16} /> },
            { id: 'themes', label: 'Themes', icon: <Palette size={16} /> },
            { id: 'security', label: 'Security', icon: <Shield size={16} /> },
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
              {tab.icon}
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Sites List */}
        <div className="lg:col-span-1">
          <div className={CARD}>
            <div className={CARD_HEADER}>
              <div className="relative">
                <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" size={16} />
                <input
                  type="text"
                  aria-label="Search WordPress sites"
                  placeholder="Search sites..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className={`${INPUT} pl-9`}
                />
              </div>
            </div>
            <div className="max-h-[600px] overflow-y-auto">
              {loading ? (
                <div className="px-5 py-10 text-center text-sm text-gray-500">Loading…</div>
              ) : filteredSites.length === 0 ? (
                <div className="px-5 py-10 text-center text-sm text-gray-500">
                  No WordPress sites found
                </div>
              ) : (
                filteredSites.map((site) => (
                  <div
                    key={site.id}
                    onClick={() => setSelectedSite(site)}
                    className={`cursor-pointer border-b border-gray-100 px-5 py-4 hover:bg-gray-50 ${
                      selectedSite?.id === site.id ? 'border-l-4 border-l-brand-600 bg-brand-50' : ''
                    }`}
                  >
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <p className="text-sm font-semibold text-gray-900">{site.domain}</p>
                        <p className="text-sm text-gray-600">{site.name || 'WordPress Site'}</p>
                      </div>
                      <span className={`${BADGE} ${getStatusColor(site.status)}`}>
                        {site.status || 'unknown'}
                      </span>
                    </div>
                    <div className="mt-2 flex items-center gap-4 text-xs text-gray-500">
                      <span>v{site.version || '—'}</span>
                      <span>{site.auto_update ? 'Auto-update ON' : 'Auto-update OFF'}</span>
                    </div>
                  </div>
                ))
              )}
            </div>
            <div className="border-t border-gray-200 px-5 py-3 text-sm text-gray-500">
              {total} site(s) total
            </div>
          </div>
        </div>

        {/* Detail Panel */}
        <div className="lg:col-span-2">
          {!selectedSite ? (
            <div className={`${CARD} px-5 py-12 text-center`}>
              <Globe className="mx-auto text-gray-400" size={40} />
              <p className="mt-4 text-sm text-gray-600">Select a WordPress site to manage</p>
            </div>
          ) : (
            <div className="space-y-6">
              {/* Site Info Card */}
              <div className={CARD}>
                <div className={`${CARD_HEADER} flex flex-wrap items-start justify-between gap-3`}>
                  <div>
                    <h2 className="text-sm font-semibold text-gray-900">{selectedSite.domain}</h2>
                    <p className="mt-1 text-sm text-gray-600">{selectedSite.name || 'WordPress Site'}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <a
                      href={`http://${selectedSite.domain}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className={BTN_SMALL}
                    >
                      <ExternalLink size={14} />
                      Visit
                    </a>
                    <a
                      href={`http://${selectedSite.domain}/wp-admin`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 rounded-md bg-brand-600 px-2.5 py-1.5 text-xs font-medium text-white hover:bg-brand-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                    >
                      <Settings size={14} />
                      WP Admin
                    </a>
                    <button
                      type="button"
                      onClick={() => handleDeleteSite(selectedSite.id)}
                      className={BTN_SMALL_DANGER}
                    >
                      <Trash2 size={14} />
                      Delete
                    </button>
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-4 p-5 md:grid-cols-4">
                  <div className="rounded-md border border-gray-200 bg-gray-50 p-3">
                    <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Version</p>
                    <p className="mt-1 text-sm font-medium text-gray-900">{selectedSite.version || '—'}</p>
                  </div>
                  <div className="rounded-md border border-gray-200 bg-gray-50 p-3">
                    <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Admin</p>
                    <p className="mt-1 text-sm font-medium text-gray-900">{selectedSite.admin_user || '—'}</p>
                  </div>
                  <div className="rounded-md border border-gray-200 bg-gray-50 p-3">
                    <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Database</p>
                    <p className="mt-1 text-sm font-medium text-gray-900">{selectedSite.db_name || '—'}</p>
                  </div>
                  <div className="rounded-md border border-gray-200 bg-gray-50 p-3">
                    <p className="text-xs font-medium uppercase tracking-wide text-gray-500">Auto Update</p>
                    <button
                      type="button"
                      onClick={() => handleToggleAutoUpdate(selectedSite)}
                      className={`mt-1 rounded-md text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 ${
                        selectedSite.auto_update ? 'text-emerald-700' : 'text-gray-600'
                      }`}
                    >
                      {selectedSite.auto_update ? 'Enabled' : 'Disabled'}
                    </button>
                  </div>
                </div>
              </div>

              {/* Tab Content */}
              {activeTab === 'sites' && (
                <div className={CARD}>
                  <div className={CARD_HEADER}>
                    <h2 className="text-sm font-semibold text-gray-900">Site Configuration</h2>
                  </div>
                  <div className="grid grid-cols-1 gap-4 p-5 md:grid-cols-2">
                    <div>
                      <p className={LABEL}>Domain</p>
                      <p className="text-sm text-gray-900">{selectedSite.domain}</p>
                    </div>
                    <div>
                      <p className={LABEL}>Admin Email</p>
                      <p className="text-sm text-gray-900">{selectedSite.admin_email || '—'}</p>
                    </div>
                    <div>
                      <p className={LABEL}>DB Host</p>
                      <p className="text-sm text-gray-900">{selectedSite.db_host || '—'}</p>
                    </div>
                    <div>
                      <p className={LABEL}>DB Prefix</p>
                      <p className="text-sm text-gray-900">{selectedSite.db_prefix || '—'}</p>
                    </div>
                    <div>
                      <p className={LABEL}>Created</p>
                      <p className="text-sm text-gray-900" suppressHydrationWarning>{formatDateTime(selectedSite.created_at)}</p>
                    </div>
                    <div>
                      <p className={LABEL}>Last Updated</p>
                      <p className="text-sm text-gray-900" suppressHydrationWarning>{formatDateTime(selectedSite.updated_at)}</p>
                    </div>
                  </div>
                </div>
              )}

              {activeTab === 'plugins' && (
                <div className={CARD}>
                  <div className={CARD_HEADER}>
                    <h2 className="text-sm font-semibold text-gray-900">Plugins ({plugins.length})</h2>
                  </div>
                  <div className="p-5">
                    {plugins.length === 0 ? (
                      <p className="py-8 text-center text-sm text-gray-500">No plugins installed</p>
                    ) : (
                      <div className="space-y-3">
                        {plugins.map((plugin) => (
                          <div key={plugin.id} className="flex items-center justify-between gap-4 rounded-md border border-gray-200 bg-gray-50 p-4">
                            <div className="flex items-center gap-3">
                              <Package className="text-gray-500" size={18} />
                              <div>
                                <p className="text-sm font-medium text-gray-900">{plugin.name}</p>
                                <p className="text-sm text-gray-600">
                                  v{plugin.version || '—'} • {plugin.author || 'Unknown author'}
                                </p>
                              </div>
                            </div>
                            <div className="flex items-center gap-2">
                              <span className={`${BADGE} ${getStatusColor(plugin.status)}`}>
                                {plugin.status || 'unknown'}
                              </span>
                              <button
                                type="button"
                                onClick={() => handleTogglePlugin(selectedSite.id, plugin)}
                                className={BTN_SMALL}
                              >
                                {plugin.status === 'active' ? 'Deactivate' : 'Activate'}
                              </button>
                              <button
                                type="button"
                                onClick={() => handleDeletePlugin(selectedSite.id, plugin.id)}
                                className={BTN_SMALL_DANGER}
                              >
                                Delete
                              </button>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}

              {activeTab === 'themes' && (
                <div className={CARD}>
                  <div className={CARD_HEADER}>
                    <h2 className="text-sm font-semibold text-gray-900">Themes ({themes.length})</h2>
                  </div>
                  <div className="p-5">
                    {themes.length === 0 ? (
                      <p className="py-8 text-center text-sm text-gray-500">No themes installed</p>
                    ) : (
                      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                        {themes.map((theme) => (
                          <div
                            key={theme.id}
                            className={`rounded-md border p-4 ${
                              theme.active ? 'border-brand-200 bg-brand-50' : 'border-gray-200 bg-white'
                            }`}
                          >
                            <div className="mb-2 flex items-center justify-between gap-2">
                              <div className="flex items-center gap-2">
                                <Palette className="text-gray-500" size={16} />
                                <p className="text-sm font-medium text-gray-900">{theme.name}</p>
                              </div>
                              {theme.active && (
                                <span className={`${BADGE} bg-brand-50 text-brand-700`}>Active</span>
                              )}
                            </div>
                            <p className="mb-3 text-sm text-gray-600">
                              v{theme.version || '—'} • {theme.author || 'Unknown'}
                            </p>
                            <div className="flex items-center gap-2">
                              {!theme.active && (
                                <button
                                  type="button"
                                  onClick={() => handleActivateTheme(selectedSite.id, theme.id)}
                                  className={BTN_SMALL}
                                >
                                  Activate
                                </button>
                              )}
                              {!theme.active && (
                                <button
                                  type="button"
                                  onClick={() => handleDeleteTheme(selectedSite.id, theme.id)}
                                  className={BTN_SMALL_DANGER}
                                >
                                  Delete
                                </button>
                              )}
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}

              {activeTab === 'security' && (
                <div className={CARD}>
                  <div className={CARD_HEADER}>
                    <h2 className="text-sm font-semibold text-gray-900">Security Check</h2>
                  </div>
                  <div className="space-y-3 p-5">
                    {[
                      { label: 'WordPress Version', status: 'ok', detail: `v${selectedSite.version} - Up to date` },
                      { label: 'Auto Updates', status: selectedSite.auto_update ? 'ok' : 'warning', detail: selectedSite.auto_update ? 'Enabled' : 'Disabled' },
                      { label: 'File Permissions', status: 'ok', detail: 'Correct permissions set' },
                      { label: 'Database Prefix', status: selectedSite.db_prefix !== 'wp_' ? 'ok' : 'warning', detail: selectedSite.db_prefix !== 'wp_' ? 'Custom prefix' : 'Default prefix (wp_)' },
                      { label: 'Admin Username', status: selectedSite.admin_user !== 'admin' ? 'ok' : 'warning', detail: selectedSite.admin_user !== 'admin' ? 'Custom username' : 'Default username (admin)' },
                    ].map((check, idx) => (
                      <div key={idx} className="flex items-center justify-between gap-4 rounded-md border border-gray-200 bg-gray-50 p-3">
                        <div className="flex items-center gap-3">
                          {check.status === 'ok' ? (
                            <CheckCircle className="text-emerald-600" size={16} />
                          ) : check.status === 'warning' ? (
                            <AlertCircle className="text-amber-600" size={16} />
                          ) : (
                            <XCircle className="text-red-600" size={16} />
                          )}
                          <span className="text-sm font-medium text-gray-900">{check.label}</span>
                        </div>
                        <span className="text-sm text-gray-600">{check.detail}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Create Site Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40 p-4">
          <div className="max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg">
            <div className="border-b border-gray-200 px-5 py-4">
              <h2 className="text-sm font-semibold text-gray-900">Create WordPress Site</h2>
            </div>
            <div className="space-y-4 p-5">
              <div>
                <label htmlFor="wp-server-id" className={LABEL}>Server ID</label>
                <input
                  id="wp-server-id"
                  type="text"
                  value={createForm.server_id}
                  onChange={(e) => setCreateForm({...createForm, server_id: e.target.value})}
                  className={INPUT}
                  placeholder="Server UUID"
                />
              </div>
              <div>
                <label htmlFor="wp-domain" className={LABEL}>Domain</label>
                <input
                  id="wp-domain"
                  type="text"
                  value={createForm.domain}
                  onChange={(e) => setCreateForm({...createForm, domain: e.target.value})}
                  className={INPUT}
                  placeholder="example.com"
                />
              </div>
              <div>
                <label htmlFor="wp-site-name" className={LABEL}>Site Name</label>
                <input
                  id="wp-site-name"
                  type="text"
                  value={createForm.name}
                  onChange={(e) => setCreateForm({...createForm, name: e.target.value})}
                  className={INPUT}
                  placeholder="My WordPress Site"
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label htmlFor="wp-admin-user" className={LABEL}>Admin User</label>
                  <input
                    id="wp-admin-user"
                    type="text"
                    value={createForm.admin_user}
                    onChange={(e) => setCreateForm({...createForm, admin_user: e.target.value})}
                    className={INPUT}
                  />
                </div>
                <div>
                  <label htmlFor="wp-admin-email" className={LABEL}>Admin Email</label>
                  <input
                    id="wp-admin-email"
                    type="email"
                    value={createForm.admin_email}
                    onChange={(e) => setCreateForm({...createForm, admin_email: e.target.value})}
                    className={INPUT}
                  />
                </div>
              </div>
              <div>
                <label htmlFor="wp-admin-password" className={LABEL}>Admin Password</label>
                <input
                  id="wp-admin-password"
                  type="password"
                  value={createForm.admin_password}
                  onChange={(e) => setCreateForm({...createForm, admin_password: e.target.value})}
                  className={INPUT}
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label htmlFor="wp-db-name" className={LABEL}>DB Name</label>
                  <input
                    id="wp-db-name"
                    type="text"
                    value={createForm.db_name}
                    onChange={(e) => setCreateForm({...createForm, db_name: e.target.value})}
                    className={INPUT}
                  />
                </div>
                <div>
                  <label htmlFor="wp-db-user" className={LABEL}>DB User</label>
                  <input
                    id="wp-db-user"
                    type="text"
                    value={createForm.db_user}
                    onChange={(e) => setCreateForm({...createForm, db_user: e.target.value})}
                    className={INPUT}
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label htmlFor="wp-db-password" className={LABEL}>DB Password</label>
                  <input
                    id="wp-db-password"
                    type="password"
                    value={createForm.db_password}
                    onChange={(e) => setCreateForm({...createForm, db_password: e.target.value})}
                    className={INPUT}
                  />
                </div>
                <div>
                  <label htmlFor="wp-db-host" className={LABEL}>DB Host</label>
                  <input
                    id="wp-db-host"
                    type="text"
                    value={createForm.db_host}
                    onChange={(e) => setCreateForm({...createForm, db_host: e.target.value})}
                    className={INPUT}
                  />
                </div>
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              <button type="button" onClick={() => setShowCreateModal(false)} className={BTN_SECONDARY}>
                Cancel
              </button>
              <button type="button" onClick={handleCreateSite} className={BTN_PRIMARY}>
                Create Site
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
