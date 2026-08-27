'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  Key,
  Plus,
  Trash2,
  Edit,
  Copy,
  Shield,
  ShieldCheck,
  ShieldX,
  Filter,
  RefreshCw,
  X,
  AlertTriangle,
  CheckCircle,
  XCircle,
  Clock,
  Eye,
  EyeOff,
  AlertOctagon,
} from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { apiKeyApi } from '@/services/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface APIKey {
  id: string;
  tenant_id: string;
  user_id: string;
  name: string;
  key_prefix: string;
  scopes: string[];
  last_used: string | null;
  expires_at: string | null;
  status: 'active' | 'expired' | 'revoked';
  created_at: string;
}

interface APIKeyFormData {
  name: string;
  scopes: string;
  expires_at: string;
}

const EMPTY_FORM: APIKeyFormData = {
  name: '',
  scopes: '',
  expires_at: '',
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function APIKeysPage() {
  // Data
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filters
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState('');

  // Modal state
  const [showForm, setShowForm] = useState(false);
  const [editingKey, setEditingKey] = useState<APIKey | null>(null);
  const [formData, setFormData] = useState<APIKeyFormData>(EMPTY_FORM);
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  // Created key modal (show full key once)
  const [createdKey, setCreatedKey] = useState<string | null>(null);
  const [showCreatedKey, setShowCreatedKey] = useState(false);
  const [copiedKey, setCopiedKey] = useState(false);

  // Revoke confirmation
  const [revokingKey, setRevokingKey] = useState<APIKey | null>(null);
  const [revoking, setRevoking] = useState(false);

  // Delete confirmation
  const [deletingKey, setDeletingKey] = useState<APIKey | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Toast
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // ---------------------------------------------------------------------------
  // Data fetching
  // ---------------------------------------------------------------------------

  const fetchKeys = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await apiKeyApi.list();
      setKeys(res.data.data || res.data || []);
    } catch (err: any) {
      console.error('Failed to load API keys:', err);
      setError(err.response?.data?.message || 'Failed to load API keys');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchKeys();
  }, [fetchKeys]);

  // Auto-dismiss toast
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 4000);
    return () => clearTimeout(t);
  }, [toast]);

  // ---------------------------------------------------------------------------
  // Computed / filtered data
  // ---------------------------------------------------------------------------

  const filteredKeys = keys.filter((k) => {
    if (statusFilter !== 'all' && k.status !== statusFilter) return false;
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      return (
        k.name.toLowerCase().includes(q) ||
        k.key_prefix.toLowerCase().includes(q) ||
        k.scopes.some((s) => s.toLowerCase().includes(q))
      );
    }
    return true;
  });

  const stats = {
    total: keys.length,
    active: keys.filter((k) => k.status === 'active').length,
    expired: keys.filter((k) => k.status === 'expired').length,
    revoked: keys.filter((k) => k.status === 'revoked').length,
  };

  // ---------------------------------------------------------------------------
  // Form helpers
  // ---------------------------------------------------------------------------

  const openCreateForm = () => {
    setEditingKey(null);
    setFormData(EMPTY_FORM);
    setFormError(null);
    setShowForm(true);
  };

  const openEditForm = (apiKey: APIKey) => {
    setEditingKey(apiKey);
    setFormData({
      name: apiKey.name,
      scopes: apiKey.scopes.join(', '),
      expires_at: apiKey.expires_at ? apiKey.expires_at.split('T')[0] : '',
    });
    setFormError(null);
    setShowForm(true);
  };

  const closeForm = () => {
    setShowForm(false);
    setEditingKey(null);
    setFormError(null);
  };

  const handleFormChange = (field: keyof APIKeyFormData, value: string) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);

    // Basic validation
    if (!formData.name.trim()) {
      setFormError('Name is required');
      return;
    }

    const scopes = formData.scopes
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);

    const payload: any = {
      name: formData.name.trim(),
      scopes,
    };

    if (formData.expires_at) {
      payload.expires_at = new Date(formData.expires_at).toISOString();
    }

    try {
      setSubmitting(true);
      if (editingKey) {
        await apiKeyApi.update(editingKey.id, payload);
        setToast({ type: 'success', message: 'API key updated successfully' });
      } else {
        const res = await apiKeyApi.create(payload);
        const newKey = res.data?.data?.key || res.data?.key;
        if (newKey) {
          setCreatedKey(newKey);
          setShowCreatedKey(true);
        }
        setToast({ type: 'success', message: 'API key created successfully' });
      }
      closeForm();
      fetchKeys();
    } catch (err: any) {
      const msg = err.response?.data?.message || 'An error occurred while saving the API key';
      setFormError(msg);
    } finally {
      setSubmitting(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Revoke helpers
  // ---------------------------------------------------------------------------

  const confirmRevoke = (apiKey: APIKey) => {
    setRevokingKey(apiKey);
  };

  const handleRevoke = async () => {
    if (!revokingKey) return;
    try {
      setRevoking(true);
      await apiKeyApi.update(revokingKey.id, { status: 'revoked' });
      setToast({ type: 'success', message: 'API key revoked successfully' });
      setRevokingKey(null);
      fetchKeys();
    } catch (err: any) {
      setToast({
        type: 'error',
        message: err.response?.data?.message || 'Failed to revoke API key',
      });
    } finally {
      setRevoking(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Delete helpers
  // ---------------------------------------------------------------------------

  const confirmDelete = (apiKey: APIKey) => {
    setDeletingKey(apiKey);
  };

  const handleDelete = async () => {
    if (!deletingKey) return;
    try {
      setDeleting(true);
      await apiKeyApi.delete(deletingKey.id);
      setToast({ type: 'success', message: 'API key deleted successfully' });
      setDeletingKey(null);
      fetchKeys();
    } catch (err: any) {
      setToast({
        type: 'error',
        message: err.response?.data?.message || 'Failed to delete API key',
      });
    } finally {
      setDeleting(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Copy helper
  // ---------------------------------------------------------------------------

  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedKey(true);
      setTimeout(() => setCopiedKey(false), 2000);
    } catch {
      // Fallback for older browsers
      const textarea = document.createElement('textarea');
      textarea.value = text;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand('copy');
      document.body.removeChild(textarea);
      setCopiedKey(true);
      setTimeout(() => setCopiedKey(false), 2000);
    }
  };

  // ---------------------------------------------------------------------------
  // Render helpers
  // ---------------------------------------------------------------------------

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'active':
        return (
          <Badge variant="success" className="flex items-center gap-1 w-fit">
            <ShieldCheck size={12} />
            Active
          </Badge>
        );
      case 'expired':
        return (
          <Badge variant="warning" className="flex items-center gap-1 w-fit">
            <Clock size={12} />
            Expired
          </Badge>
        );
      case 'revoked':
        return (
          <Badge variant="destructive" className="flex items-center gap-1 w-fit">
            <ShieldX size={12} />
            Revoked
          </Badge>
        );
      default:
        return (
          <Badge variant="secondary" className="flex items-center gap-1 w-fit">
            {status}
          </Badge>
        );
    }
  };

  const formatDate = (dateStr: string | null) => {
    if (!dateStr) return 'N/A';
    return new Date(dateStr).toLocaleString();
  };

  const isExpired = (expiresAt: string | null) => {
    if (!expiresAt) return false;
    return new Date(expiresAt) < new Date();
  };

  // ---------------------------------------------------------------------------
  // Loading state
  // ---------------------------------------------------------------------------

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="h-8 w-8 animate-spin text-primary-500" />
      </div>
    );
  }

  // ---------------------------------------------------------------------------
  // Main render
  // ---------------------------------------------------------------------------

  return (
    <div className="space-y-6">
      {/* Toast */}
      {toast && (
        <div
          className={`fixed top-4 right-4 z-50 flex items-center gap-3 px-4 py-3 rounded-lg shadow-lg border ${
            toast.type === 'success'
              ? 'bg-green-900/90 border-green-700 text-green-100'
              : 'bg-red-900/90 border-red-700 text-red-100'
          }`}
        >
          {toast.type === 'success' ? <CheckCircle size={18} /> : <XCircle size={18} />}
          <span className="text-sm font-medium">{toast.message}</span>
          <button onClick={() => setToast(null)} className="ml-2 hover:opacity-70">
            <X size={14} />
          </button>
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50 flex items-center gap-2">
            <Key size={28} className="text-primary-400" />
            API Keys
          </h1>
          <p className="text-dark-400 mt-1">
            Manage API keys for programmatic access
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            onClick={fetchKeys}
            className="border-dark-600 text-dark-300 hover:bg-dark-700"
          >
            <RefreshCw size={16} className="mr-2" />
            Refresh
          </Button>
          <Button
            onClick={openCreateForm}
            className="bg-primary-600 hover:bg-primary-700 text-white"
          >
            <Plus size={16} className="mr-2" />
            Create API Key
          </Button>
        </div>
      </div>

      {/* Error banner */}
      {error && (
        <div className="flex items-center gap-3 px-4 py-3 rounded-lg bg-red-900/30 border border-red-700 text-red-300">
          <AlertTriangle size={18} />
          <span className="text-sm">{error}</span>
          <button onClick={() => setError(null)} className="ml-auto hover:opacity-70">
            <X size={14} />
          </button>
        </div>
      )}

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card className="bg-dark-800 border-dark-700">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-dark-300">Total Keys</CardTitle>
            <Key className="h-4 w-4 text-dark-400" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-dark-50">{stats.total}</div>
            <p className="text-xs text-dark-500 mt-1">All API keys</p>
          </CardContent>
        </Card>

        <Card className="bg-dark-800 border-dark-700">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-dark-300">Active</CardTitle>
            <ShieldCheck className="h-4 w-4 text-green-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-400">{stats.active}</div>
            <p className="text-xs text-dark-500 mt-1">Ready to use</p>
          </CardContent>
        </Card>

        <Card className="bg-dark-800 border-dark-700">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-dark-300">Expired</CardTitle>
            <Clock className="h-4 w-4 text-yellow-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-yellow-400">{stats.expired}</div>
            <p className="text-xs text-dark-500 mt-1">Past expiration date</p>
          </CardContent>
        </Card>

        <Card className="bg-dark-800 border-dark-700">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-dark-300">Revoked</CardTitle>
            <ShieldX className="h-4 w-4 text-red-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-red-400">{stats.revoked}</div>
            <p className="text-xs text-dark-500 mt-1">Access revoked</p>
          </CardContent>
        </Card>
      </div>

      {/* Filters */}
      <Card className="bg-dark-800 border-dark-700">
        <CardContent className="pt-6">
          <div className="flex flex-col md:flex-row gap-4">
            <div className="flex-1 relative">
              <Filter className="absolute left-3 top-1/2 -translate-y-1/2 text-dark-400" size={16} />
              <Input
                placeholder="Search by name, prefix, or scope..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-10 bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
              />
            </div>
            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger className="w-[160px] bg-dark-900 border-dark-600 text-dark-200">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent className="bg-dark-800 border-dark-600">
                <SelectItem value="all">All Statuses</SelectItem>
                <SelectItem value="active">Active</SelectItem>
                <SelectItem value="expired">Expired</SelectItem>
                <SelectItem value="revoked">Revoked</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* Keys Table */}
      <Card className="bg-dark-800 border-dark-700">
        <CardHeader>
          <CardTitle className="text-dark-100 flex items-center justify-between">
            <span>API Keys</span>
            <span className="text-sm font-normal text-dark-400">
              {filteredKeys.length} of {keys.length} keys
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {filteredKeys.length === 0 ? (
            <div className="text-center py-16">
              <Key className="mx-auto text-dark-600" size={64} />
              <h3 className="mt-4 text-xl font-medium text-dark-300">
                {keys.length === 0 ? 'No API keys' : 'No matching keys'}
              </h3>
              <p className="mt-2 text-dark-500">
                {keys.length === 0
                  ? 'Create your first API key to get started'
                  : 'Try adjusting your filters or search query'}
              </p>
              {keys.length === 0 && (
                <Button
                  onClick={openCreateForm}
                  className="mt-4 bg-primary-600 hover:bg-primary-700 text-white"
                >
                  <Plus size={16} className="mr-2" />
                  Create API Key
                </Button>
              )}
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-dark-700">
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Name</th>
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Key Prefix</th>
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Scopes</th>
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Status</th>
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Last Used</th>
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Expires At</th>
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Created At</th>
                    <th className="text-right p-3 text-sm font-medium text-dark-400">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredKeys.map((apiKey) => (
                    <tr
                      key={apiKey.id}
                      className="border-b border-dark-700/50 hover:bg-dark-700/30 transition-colors"
                    >
                      <td className="p-3">
                        <div className="flex items-center gap-2">
                          <Key size={14} className="text-primary-400" />
                          <span className="text-sm font-medium text-dark-100">
                            {apiKey.name}
                          </span>
                        </div>
                      </td>
                      <td className="p-3">
                        <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-md bg-dark-700 text-dark-200 text-sm font-mono">
                          {apiKey.key_prefix}••••••••
                        </span>
                      </td>
                      <td className="p-3">
                        <div className="flex flex-wrap gap-1">
                          {apiKey.scopes.length > 0 ? (
                            apiKey.scopes.map((scope, idx) => (
                              <Badge
                                key={idx}
                                variant="secondary"
                                className="text-xs bg-dark-700 text-dark-300 border-dark-600"
                              >
                                {scope}
                              </Badge>
                            ))
                          ) : (
                            <span className="text-xs text-dark-500">No scopes</span>
                          )}
                        </div>
                      </td>
                      <td className="p-3">{getStatusBadge(apiKey.status)}</td>
                      <td className="p-3 text-sm text-dark-400">
                        {formatDate(apiKey.last_used)}
                      </td>
                      <td className="p-3">
                        <span
                          className={`text-sm ${
                            isExpired(apiKey.expires_at)
                              ? 'text-red-400'
                              : apiKey.expires_at
                              ? 'text-dark-400'
                              : 'text-dark-500'
                          }`}
                        >
                          {apiKey.expires_at ? formatDate(apiKey.expires_at) : 'Never'}
                        </span>
                      </td>
                      <td className="p-3 text-sm text-dark-400">
                        {formatDate(apiKey.created_at)}
                      </td>
                      <td className="p-3">
                        <div className="flex items-center justify-end gap-1">
                          {apiKey.status === 'active' && (
                            <>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => openEditForm(apiKey)}
                                className="text-dark-300 hover:text-dark-100 hover:bg-dark-700"
                                title="Edit"
                              >
                                <Edit size={14} />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => confirmRevoke(apiKey)}
                                className="text-yellow-400 hover:text-yellow-300 hover:bg-yellow-900/20"
                                title="Revoke"
                              >
                                <AlertOctagon size={14} />
                              </Button>
                            </>
                          )}
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => confirmDelete(apiKey)}
                            className="text-red-400 hover:text-red-300 hover:bg-red-900/20"
                            title="Delete"
                          >
                            <Trash2 size={14} />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* ------------------------------------------------------------------ */}
      {/* Create / Edit Modal                                                */}
      {/* ------------------------------------------------------------------ */}
      {showForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          {/* Backdrop */}
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={closeForm}
          />

          {/* Modal */}
          <div className="relative w-full max-w-lg mx-4 bg-dark-800 border border-dark-600 rounded-xl shadow-2xl">
            {/* Header */}
            <div className="flex items-center justify-between px-6 py-4 border-b border-dark-700">
              <h2 className="text-lg font-semibold text-dark-50 flex items-center gap-2">
                <Key size={20} className="text-primary-400" />
                {editingKey ? 'Edit API Key' : 'Create API Key'}
              </h2>
              <button
                onClick={closeForm}
                className="text-dark-400 hover:text-dark-200 transition-colors"
              >
                <X size={20} />
              </button>
            </div>

            {/* Form */}
            <form onSubmit={handleSubmit} className="px-6 py-4 space-y-4">
              {formError && (
                <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-red-900/30 border border-red-700 text-red-300 text-sm">
                  <AlertTriangle size={14} />
                  {formError}
                </div>
              )}

              {/* Name */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1.5">
                  Name <span className="text-red-400">*</span>
                </label>
                <Input
                  placeholder="e.g. CI/CD Pipeline, Monitoring Agent"
                  value={formData.name}
                  onChange={(e) => handleFormChange('name', e.target.value)}
                  className="bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
                  required
                />
              </div>

              {/* Scopes */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1.5">
                  Scopes
                </label>
                <Input
                  placeholder="e.g. read, write, admin (comma-separated)"
                  value={formData.scopes}
                  onChange={(e) => handleFormChange('scopes', e.target.value)}
                  className="bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
                />
                <p className="text-xs text-dark-500 mt-1">
                  Comma-separated list of permission scopes
                </p>
              </div>

              {/* Expiration */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1.5">
                  Expiration Date
                </label>
                <Input
                  type="date"
                  value={formData.expires_at}
                  onChange={(e) => handleFormChange('expires_at', e.target.value)}
                  className="bg-dark-900 border-dark-600 text-dark-100 focus:border-primary-500"
                  min={new Date().toISOString().split('T')[0]}
                />
                <p className="text-xs text-dark-500 mt-1">
                  Leave empty for no expiration
                </p>
              </div>

              {/* Actions */}
              <div className="flex items-center justify-end gap-3 pt-4 border-t border-dark-700">
                <Button
                  type="button"
                  variant="outline"
                  onClick={closeForm}
                  className="border-dark-600 text-dark-300 hover:bg-dark-700"
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={submitting}
                  className="bg-primary-600 hover:bg-primary-700 text-white disabled:opacity-50"
                >
                  {submitting ? (
                    <RefreshCw size={16} className="mr-2 animate-spin" />
                  ) : (
                    <Key size={16} className="mr-2" />
                  )}
                  {editingKey ? 'Update Key' : 'Create Key'}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ------------------------------------------------------------------ */}
      {/* Created Key Modal (show full key once)                             */}
      {/* ------------------------------------------------------------------ */}
      {createdKey && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          {/* Backdrop */}
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => setCreatedKey(null)}
          />

          {/* Modal */}
          <div className="relative w-full max-w-lg mx-4 bg-dark-800 border border-dark-600 rounded-xl shadow-2xl">
            {/* Header */}
            <div className="flex items-center justify-between px-6 py-4 border-b border-dark-700">
              <h2 className="text-lg font-semibold text-dark-50 flex items-center gap-2">
                <ShieldCheck size={20} className="text-green-400" />
                API Key Created
              </h2>
              <button
                onClick={() => setCreatedKey(null)}
                className="text-dark-400 hover:text-dark-200 transition-colors"
              >
                <X size={20} />
              </button>
            </div>

            <div className="px-6 py-4 space-y-4">
              {/* Warning */}
              <div className="flex items-start gap-3 px-4 py-3 rounded-lg bg-yellow-900/30 border border-yellow-700 text-yellow-200">
                <AlertTriangle size={18} className="mt-0.5 flex-shrink-0" />
                <div>
                  <p className="text-sm font-medium">Save this key now!</p>
                  <p className="text-xs text-yellow-300 mt-1">
                    This is the only time the full API key will be displayed. Copy it now and store
                    it securely — it cannot be retrieved again.
                  </p>
                </div>
              </div>

              {/* Key display */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1.5">
                  Your API Key
                </label>
                <div className="relative">
                  <div className="flex items-center gap-2">
                    <div className="flex-1 px-3 py-2.5 bg-dark-900 border border-dark-600 rounded-md font-mono text-sm text-dark-100 break-all select-all">
                      {showCreatedKey ? createdKey : '•'.repeat(Math.min(createdKey.length, 40))}
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => setShowCreatedKey(!showCreatedKey)}
                      className="text-dark-400 hover:text-dark-200 hover:bg-dark-700 flex-shrink-0"
                      title={showCreatedKey ? 'Hide key' : 'Show key'}
                    >
                      {showCreatedKey ? <EyeOff size={16} /> : <Eye size={16} />}
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => copyToClipboard(createdKey)}
                      className={`flex-shrink-0 ${
                        copiedKey
                          ? 'text-green-400 hover:text-green-300'
                          : 'text-dark-400 hover:text-dark-200'
                      } hover:bg-dark-700`}
                      title="Copy to clipboard"
                    >
                      {copiedKey ? <CheckCircle size={16} /> : <Copy size={16} />}
                    </Button>
                  </div>
                </div>
                {copiedKey && (
                  <p className="text-xs text-green-400 mt-1.5">Copied to clipboard!</p>
                )}
              </div>

              {/* Close */}
              <div className="flex justify-end pt-2">
                <Button
                  onClick={() => setCreatedKey(null)}
                  className="bg-primary-600 hover:bg-primary-700 text-white"
                >
                  I&apos;ve saved my key
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ------------------------------------------------------------------ */}
      {/* Revoke Confirmation Modal                                          */}
      {/* ------------------------------------------------------------------ */}
      {revokingKey && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => setRevokingKey(null)}
          />
          <div className="relative w-full max-w-md mx-4 bg-dark-800 border border-dark-600 rounded-xl shadow-2xl">
            <div className="px-6 py-4 border-b border-dark-700">
              <h2 className="text-lg font-semibold text-dark-50 flex items-center gap-2">
                <AlertOctagon size={20} className="text-yellow-400" />
                Revoke API Key
              </h2>
            </div>
            <div className="px-6 py-4 space-y-4">
              <p className="text-sm text-dark-300">
                Are you sure you want to revoke the API key{' '}
                <span className="font-semibold text-dark-100">&quot;{revokingKey.name}&quot;</span>?
                This action will immediately invalidate the key and any applications using it will
                lose access.
              </p>
              <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-yellow-900/20 border border-yellow-700/50 text-yellow-300 text-sm">
                <AlertTriangle size={14} />
                This action cannot be undone.
              </div>
              <div className="flex items-center justify-end gap-3 pt-2">
                <Button
                  variant="outline"
                  onClick={() => setRevokingKey(null)}
                  className="border-dark-600 text-dark-300 hover:bg-dark-700"
                >
                  Cancel
                </Button>
                <Button
                  onClick={handleRevoke}
                  disabled={revoking}
                  className="bg-yellow-600 hover:bg-yellow-700 text-white disabled:opacity-50"
                >
                  {revoking ? (
                    <RefreshCw size={16} className="mr-2 animate-spin" />
                  ) : (
                    <AlertOctagon size={16} className="mr-2" />
                  )}
                  Revoke Key
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ------------------------------------------------------------------ */}
      {/* Delete Confirmation Modal                                          */}
      {/* ------------------------------------------------------------------ */}
      {deletingKey && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => setDeletingKey(null)}
          />
          <div className="relative w-full max-w-md mx-4 bg-dark-800 border border-dark-600 rounded-xl shadow-2xl">
            <div className="px-6 py-4 border-b border-dark-700">
              <h2 className="text-lg font-semibold text-dark-50 flex items-center gap-2">
                <Trash2 size={20} className="text-red-400" />
                Delete API Key
              </h2>
            </div>
            <div className="px-6 py-4 space-y-4">
              <p className="text-sm text-dark-300">
                Are you sure you want to permanently delete the API key{' '}
                <span className="font-semibold text-dark-100">&quot;{deletingKey.name}&quot;</span>?
                This will remove the key record entirely.
              </p>
              <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-red-900/20 border border-red-700/50 text-red-300 text-sm">
                <AlertTriangle size={14} />
                This action is permanent and cannot be undone.
              </div>
              <div className="flex items-center justify-end gap-3 pt-2">
                <Button
                  variant="outline"
                  onClick={() => setDeletingKey(null)}
                  className="border-dark-600 text-dark-300 hover:bg-dark-700"
                >
                  Cancel
                </Button>
                <Button
                  onClick={handleDelete}
                  disabled={deleting}
                  className="bg-red-600 hover:bg-red-700 text-white disabled:opacity-50"
                >
                  {deleting ? (
                    <RefreshCw size={16} className="mr-2 animate-spin" />
                  ) : (
                    <Trash2 size={16} className="mr-2" />
                  )}
                  Delete Key
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
