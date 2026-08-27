'use client';

import { useState, useEffect } from 'react';
import { 
  Globe, Plus, Settings, Trash2, RefreshCw, ExternalLink,
  Package, Palette, Shield, CheckCircle, XCircle, AlertCircle,
  ChevronDown, ChevronRight, Search, Download, Upload, Lock,
  Unlock, Eye, EyeOff, Server, Database, HardDrive
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

export default function WordPressToolkitPage() {
  const [sites, setSites] = useState<WordPressSite[]>([]);
  const [selectedSite, setSelectedSite] = useState<WordPressSite | null>(null);
  const [plugins, setPlugins] = useState<WordPressPlugin[]>([]);
  const [themes, setThemes] = useState<WordPressTheme[]>([]);
  const [loading, setLoading] = useState(true);
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
    try {
      const token = localStorage.getItem('token');
      const res = await fetch('/api/v1/wordpress?limit=100&offset=0', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setSites(data.sites || []);
        setTotal(data.total || 0);
      }
    } catch (error) {
      console.error('Failed to fetch WordPress sites:', error);
    } finally {
      setLoading(false);
    }
  };

  const fetchPlugins = async (siteId: string) => {
    try {
      const token = localStorage.getItem('token');
      const res = await fetch(`/api/v1/wordpress/${siteId}/plugins`, {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setPlugins(data.plugins || []);
      }
    } catch (error) {
      console.error('Failed to fetch plugins:', error);
    }
  };

  const fetchThemes = async (siteId: string) => {
    try {
      const token = localStorage.getItem('token');
      const res = await fetch(`/api/v1/wordpress/${siteId}/themes`, {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setThemes(data.themes || []);
      }
    } catch (error) {
      console.error('Failed to fetch themes:', error);
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
      const token = localStorage.getItem('token');
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
      }
    } catch (error) {
      console.error('Failed to create WordPress site:', error);
    }
  };

  const handleDeleteSite = async (siteId: string) => {
    if (!confirm('Are you sure you want to delete this WordPress site? This action cannot be undone.')) return;
    try {
      const token = localStorage.getItem('token');
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
    }
  };

  const handleToggleAutoUpdate = async (site: WordPressSite) => {
    try {
      const token = localStorage.getItem('token');
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
    }
  };

  const handleTogglePlugin = async (siteId: string, plugin: WordPressPlugin) => {
    try {
      const token = localStorage.getItem('token');
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
    }
  };

  const handleDeletePlugin = async (siteId: string, pluginId: string) => {
    if (!confirm('Are you sure you want to delete this plugin?')) return;
    try {
      const token = localStorage.getItem('token');
      const res = await fetch(`/api/v1/wordpress/${siteId}/plugins/${pluginId}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok && selectedSite) fetchPlugins(selectedSite.id);
    } catch (error) {
      console.error('Failed to delete plugin:', error);
    }
  };

  const handleActivateTheme = async (siteId: string, themeId: string) => {
    try {
      const token = localStorage.getItem('token');
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
    }
  };

  const handleDeleteTheme = async (siteId: string, themeId: string) => {
    if (!confirm('Are you sure you want to delete this theme?')) return;
    try {
      const token = localStorage.getItem('token');
      const res = await fetch(`/api/v1/wordpress/${siteId}/themes/${themeId}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok && selectedSite) fetchThemes(selectedSite.id);
    } catch (error) {
      console.error('Failed to delete theme:', error);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'active': return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400';
      case 'inactive': return 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300';
      case 'error': return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400';
      default: return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400';
    }
  };

  const filteredSites = sites.filter(s => 
    s.domain.toLowerCase().includes(searchTerm.toLowerCase()) ||
    s.name.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
            <Globe className="text-blue-500" size={28} />
            WordPress Toolkit Pro
          </h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">
            Manage WordPress installations, plugins, themes, and security
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={fetchSites}
            className="px-4 py-2 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600 flex items-center gap-2"
          >
            <RefreshCw size={16} />
            Refresh
          </button>
          <button
            onClick={() => setShowCreateModal(true)}
            className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 flex items-center gap-2"
          >
            <Plus size={16} />
            New Site
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex space-x-1 bg-gray-100 dark:bg-gray-800 p-1 rounded-lg w-fit">
        {[
          { id: 'sites', label: 'Sites', icon: <Globe size={16} /> },
          { id: 'plugins', label: 'Plugins', icon: <Package size={16} /> },
          { id: 'themes', label: 'Themes', icon: <Palette size={16} /> },
          { id: 'security', label: 'Security', icon: <Shield size={16} /> },
        ].map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id as any)}
            className={`px-4 py-2 rounded-md text-sm font-medium flex items-center gap-2 transition-colors ${
              activeTab === tab.id
                ? 'bg-white dark:bg-gray-700 text-blue-600 dark:text-blue-400 shadow-sm'
                : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
            }`}
          >
            {tab.icon}
            {tab.label}
          </button>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Sites List */}
        <div className="lg:col-span-1">
          <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700">
            <div className="p-4 border-b border-gray-200 dark:border-gray-700">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400" size={16} />
                <input
                  type="text"
                  placeholder="Search sites..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="w-full pl-10 pr-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                />
              </div>
            </div>
            <div className="divide-y divide-gray-100 dark:divide-gray-700 max-h-[600px] overflow-y-auto">
              {loading ? (
                <div className="flex items-center justify-center p-8">
                  <RefreshCw className="animate-spin text-blue-500" size={24} />
                </div>
              ) : filteredSites.length === 0 ? (
                <div className="p-8 text-center text-gray-500 dark:text-gray-400">
                  No WordPress sites found
                </div>
              ) : (
                filteredSites.map((site) => (
                  <div
                    key={site.id}
                    onClick={() => setSelectedSite(site)}
                    className={`p-4 cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors ${
                      selectedSite?.id === site.id ? 'bg-blue-50 dark:bg-blue-900/20 border-l-4 border-blue-500' : ''
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <div>
                        <p className="font-medium text-gray-900 dark:text-white">{site.domain}</p>
                        <p className="text-sm text-gray-500 dark:text-gray-400">{site.name || 'WordPress Site'}</p>
                      </div>
                      <span className={`px-2 py-1 text-xs rounded-full ${getStatusColor(site.status)}`}>
                        {site.status}
                      </span>
                    </div>
                    <div className="mt-2 flex items-center gap-4 text-xs text-gray-500 dark:text-gray-400">
                      <span>v{site.version}</span>
                      <span>{site.auto_update ? 'Auto-update ON' : 'Auto-update OFF'}</span>
                    </div>
                  </div>
                ))
              )}
            </div>
            <div className="p-3 border-t border-gray-200 dark:border-gray-700 text-sm text-gray-500 dark:text-gray-400">
              {total} site(s) total
            </div>
          </div>
        </div>

        {/* Detail Panel */}
        <div className="lg:col-span-2">
          {!selectedSite ? (
            <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 p-12 text-center">
              <Globe className="mx-auto text-gray-300 dark:text-gray-600" size={48} />
              <p className="mt-4 text-gray-500 dark:text-gray-400">Select a WordPress site to manage</p>
            </div>
          ) : (
            <div className="space-y-6">
              {/* Site Info Card */}
              <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 p-6">
                <div className="flex items-center justify-between mb-4">
                  <div>
                    <h2 className="text-xl font-bold text-gray-900 dark:text-white">{selectedSite.domain}</h2>
                    <p className="text-gray-500 dark:text-gray-400">{selectedSite.name || 'WordPress Site'}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <a
                      href={`http://${selectedSite.domain}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="px-3 py-2 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600 flex items-center gap-2 text-sm"
                    >
                      <ExternalLink size={14} />
                      Visit
                    </a>
                    <a
                      href={`http://${selectedSite.domain}/wp-admin`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="px-3 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 flex items-center gap-2 text-sm"
                    >
                      <Settings size={14} />
                      WP Admin
                    </a>
                    <button
                      onClick={() => handleDeleteSite(selectedSite.id)}
                      className="px-3 py-2 bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400 rounded-lg hover:bg-red-200 dark:hover:bg-red-900/50 flex items-center gap-2 text-sm"
                    >
                      <Trash2 size={14} />
                      Delete
                    </button>
                  </div>
                </div>

                <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                  <div className="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-3">
                    <p className="text-xs text-gray-500 dark:text-gray-400">Version</p>
                    <p className="text-sm font-medium text-gray-900 dark:text-white">{selectedSite.version}</p>
                  </div>
                  <div className="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-3">
                    <p className="text-xs text-gray-500 dark:text-gray-400">Admin</p>
                    <p className="text-sm font-medium text-gray-900 dark:text-white">{selectedSite.admin_user}</p>
                  </div>
                  <div className="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-3">
                    <p className="text-xs text-gray-500 dark:text-gray-400">Database</p>
                    <p className="text-sm font-medium text-gray-900 dark:text-white">{selectedSite.db_name}</p>
                  </div>
                  <div className="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-3">
                    <p className="text-xs text-gray-500 dark:text-gray-400">Auto Update</p>
                    <button
                      onClick={() => handleToggleAutoUpdate(selectedSite)}
                      className={`text-sm font-medium ${selectedSite.auto_update ? 'text-green-600 dark:text-green-400' : 'text-gray-500'}`}
                    >
                      {selectedSite.auto_update ? 'Enabled' : 'Disabled'}
                    </button>
                  </div>
                </div>
              </div>

              {/* Tab Content */}
              {activeTab === 'sites' && (
                <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 p-6">
                  <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Site Configuration</h3>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Domain</label>
                      <p className="text-gray-900 dark:text-white">{selectedSite.domain}</p>
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Admin Email</label>
                      <p className="text-gray-900 dark:text-white">{selectedSite.admin_email}</p>
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">DB Host</label>
                      <p className="text-gray-900 dark:text-white">{selectedSite.db_host}</p>
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">DB Prefix</label>
                      <p className="text-gray-900 dark:text-white">{selectedSite.db_prefix}</p>
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Created</label>
                      <p className="text-gray-900 dark:text-white">{new Date(selectedSite.created_at).toLocaleString()}</p>
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Last Updated</label>
                      <p className="text-gray-900 dark:text-white">{new Date(selectedSite.updated_at).toLocaleString()}</p>
                    </div>
                  </div>
                </div>
              )}

              {activeTab === 'plugins' && (
                <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 p-6">
                  <div className="flex items-center justify-between mb-4">
                    <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Plugins ({plugins.length})</h3>
                  </div>
                  {plugins.length === 0 ? (
                    <p className="text-gray-500 dark:text-gray-400 text-center py-8">No plugins installed</p>
                  ) : (
                    <div className="space-y-3">
                      {plugins.map((plugin) => (
                        <div key={plugin.id} className="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
                          <div className="flex items-center gap-3">
                            <Package className="text-blue-500" size={20} />
                            <div>
                              <p className="font-medium text-gray-900 dark:text-white">{plugin.name}</p>
                              <p className="text-sm text-gray-500 dark:text-gray-400">
                                v{plugin.version} • {plugin.author || 'Unknown author'}
                              </p>
                            </div>
                          </div>
                          <div className="flex items-center gap-2">
                            <span className={`px-2 py-1 text-xs rounded-full ${getStatusColor(plugin.status)}`}>
                              {plugin.status}
                            </span>
                            <button
                              onClick={() => handleTogglePlugin(selectedSite.id, plugin)}
                              className={`px-3 py-1 text-xs rounded-lg ${
                                plugin.status === 'active' 
                                  ? 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400' 
                                  : 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                              }`}
                            >
                              {plugin.status === 'active' ? 'Deactivate' : 'Activate'}
                            </button>
                            <button
                              onClick={() => handleDeletePlugin(selectedSite.id, plugin.id)}
                              className="px-3 py-1 text-xs rounded-lg bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400"
                            >
                              Delete
                            </button>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}

              {activeTab === 'themes' && (
                <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 p-6">
                  <div className="flex items-center justify-between mb-4">
                    <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Themes ({themes.length})</h3>
                  </div>
                  {themes.length === 0 ? (
                    <p className="text-gray-500 dark:text-gray-400 text-center py-8">No themes installed</p>
                  ) : (
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      {themes.map((theme) => (
                        <div key={theme.id} className={`p-4 rounded-lg border ${
                          theme.active 
                            ? 'border-blue-300 dark:border-blue-700 bg-blue-50 dark:bg-blue-900/20' 
                            : 'border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800'
                        }`}>
                          <div className="flex items-center justify-between mb-2">
                            <div className="flex items-center gap-2">
                              <Palette className="text-purple-500" size={18} />
                              <p className="font-medium text-gray-900 dark:text-white">{theme.name}</p>
                            </div>
                            {theme.active && (
                              <span className="px-2 py-1 text-xs rounded-full bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">
                                Active
                              </span>
                            )}
                          </div>
                          <p className="text-sm text-gray-500 dark:text-gray-400 mb-3">
                            v{theme.version} • {theme.author || 'Unknown'}
                          </p>
                          <div className="flex items-center gap-2">
                            {!theme.active && (
                              <button
                                onClick={() => handleActivateTheme(selectedSite.id, theme.id)}
                                className="px-3 py-1 text-xs rounded-lg bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
                              >
                                Activate
                              </button>
                            )}
                            {!theme.active && (
                              <button
                                onClick={() => handleDeleteTheme(selectedSite.id, theme.id)}
                                className="px-3 py-1 text-xs rounded-lg bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400"
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
              )}

              {activeTab === 'security' && (
                <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 p-6">
                  <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Security Check</h3>
                  <div className="space-y-4">
                    {[
                      { label: 'WordPress Version', status: 'ok', detail: `v${selectedSite.version} - Up to date` },
                      { label: 'Auto Updates', status: selectedSite.auto_update ? 'ok' : 'warning', detail: selectedSite.auto_update ? 'Enabled' : 'Disabled' },
                      { label: 'File Permissions', status: 'ok', detail: 'Correct permissions set' },
                      { label: 'Database Prefix', status: selectedSite.db_prefix !== 'wp_' ? 'ok' : 'warning', detail: selectedSite.db_prefix !== 'wp_' ? 'Custom prefix' : 'Default prefix (wp_)' },
                      { label: 'Admin Username', status: selectedSite.admin_user !== 'admin' ? 'ok' : 'warning', detail: selectedSite.admin_user !== 'admin' ? 'Custom username' : 'Default username (admin)' },
                    ].map((check, idx) => (
                      <div key={idx} className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
                        <div className="flex items-center gap-3">
                          {check.status === 'ok' ? (
                            <CheckCircle className="text-green-500" size={18} />
                          ) : check.status === 'warning' ? (
                            <AlertCircle className="text-yellow-500" size={18} />
                          ) : (
                            <XCircle className="text-red-500" size={18} />
                          )}
                          <span className="text-sm font-medium text-gray-900 dark:text-white">{check.label}</span>
                        </div>
                        <span className="text-sm text-gray-500 dark:text-gray-400">{check.detail}</span>
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
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-xl p-6 w-full max-w-lg max-h-[90vh] overflow-y-auto">
            <h2 className="text-xl font-bold text-gray-900 dark:text-white mb-4">Create WordPress Site</h2>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Server ID</label>
                <input
                  type="text"
                  value={createForm.server_id}
                  onChange={(e) => setCreateForm({...createForm, server_id: e.target.value})}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  placeholder="Server UUID"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Domain</label>
                <input
                  type="text"
                  value={createForm.domain}
                  onChange={(e) => setCreateForm({...createForm, domain: e.target.value})}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  placeholder="example.com"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Site Name</label>
                <input
                  type="text"
                  value={createForm.name}
                  onChange={(e) => setCreateForm({...createForm, name: e.target.value})}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  placeholder="My WordPress Site"
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Admin User</label>
                  <input
                    type="text"
                    value={createForm.admin_user}
                    onChange={(e) => setCreateForm({...createForm, admin_user: e.target.value})}
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Admin Email</label>
                  <input
                    type="email"
                    value={createForm.admin_email}
                    onChange={(e) => setCreateForm({...createForm, admin_email: e.target.value})}
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  />
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Admin Password</label>
                <input
                  type="password"
                  value={createForm.admin_password}
                  onChange={(e) => setCreateForm({...createForm, admin_password: e.target.value})}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">DB Name</label>
                  <input
                    type="text"
                    value={createForm.db_name}
                    onChange={(e) => setCreateForm({...createForm, db_name: e.target.value})}
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">DB User</label>
                  <input
                    type="text"
                    value={createForm.db_user}
                    onChange={(e) => setCreateForm({...createForm, db_user: e.target.value})}
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">DB Password</label>
                  <input
                    type="password"
                    value={createForm.db_password}
                    onChange={(e) => setCreateForm({...createForm, db_password: e.target.value})}
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">DB Host</label>
                  <input
                    type="text"
                    value={createForm.db_host}
                    onChange={(e) => setCreateForm({...createForm, db_host: e.target.value})}
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  />
                </div>
              </div>
            </div>
            <div className="flex justify-end gap-3 mt-6">
              <button
                onClick={() => setShowCreateModal(false)}
                className="px-4 py-2 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-600"
              >
                Cancel
              </button>
              <button
                onClick={handleCreateSite}
                className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600"
              >
                Create Site
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
