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
// Shared class tokens
// ---------------------------------------------------------------------------

const INPUT_CLASS =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus-visible:ring-1 focus-visible:ring-brand-500 focus-visible:ring-offset-0 focus:outline-none';
const TH_CLASS =
  'text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500';
const LABEL_CLASS = 'block text-sm font-medium text-gray-700 mb-1.5';

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

  // Today (computed client-side only, avoids hydration mismatch)
  const [todayStr, setTodayStr] = useState('');
  const [nowMs, setNowMs] = useState(0);

  // ---------------------------------------------------------------------------
  // Data fetching
  // ---------------------------------------------------------------------------

  const fetchKeys = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await apiKeyApi.list();
      const payload = res?.data?.data ?? res?.data;
      setKeys(Array.isArray(payload) ? payload : []);
    } catch (err: any) {
      console.error('Failed to load API keys:', err);
      setKeys([]);
      setError(err?.response?.data?.message || 'Failed to load API keys');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchKeys();
  }, [fetchKeys]);

  useEffect(() => {
    const now = new Date();
    setNowMs(now.getTime());
    setTodayStr(now.toISOString().split('T')[0]);
  }, []);

  // Auto-dismiss toast
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 4000);
    return () => clearTimeout(t);
  }, [toast]);

  // ---------------------------------------------------------------------------
  // Computed / filtered data
  // ---------------------------------------------------------------------------

  const safeKeys: APIKey[] = Array.isArray(keys) ? keys : [];

  const filteredKeys = safeKeys.filter((k) => {
    if (!k) return false;
    if (statusFilter !== 'all' && k.status !== statusFilter) return false;
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      const scopes = Array.isArray(k.scopes) ? k.scopes : [];
      return (
        (k.name || '').toLowerCase().includes(q) ||
        (k.key_prefix || '').toLowerCase().includes(q) ||
        scopes.some((s) => (s || '').toLowerCase().includes(q))
      );
    }
    return true;
  });

  const stats = {
    total: safeKeys.length,
    active: safeKeys.filter((k) => k?.status === 'active').length,
    expired: safeKeys.filter((k) => k?.status === 'expired').length,
    revoked: safeKeys.filter((k) => k?.status === 'revoked').length,
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
      name: apiKey?.name || '',
      scopes: Array.isArray(apiKey?.scopes) ? apiKey.scopes.join(', ') : '',
      expires_at: apiKey?.expires_at ? apiKey.expires_at.split('T')[0] : '',
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
        const newKey = res?.data?.data?.key || res?.data?.key;
        if (newKey) {
          setCreatedKey(newKey);
          setShowCreatedKey(true);
        }
        setToast({ type: 'success', message: 'API key created successfully' });
      }
      closeForm();
      fetchKeys();
    } catch (err: any) {
      const msg = err?.response?.data?.message || 'An error occurred while saving the API key';
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
        message: err?.response?.data?.message || 'Failed to revoke API key',
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
        message: err?.response?.data?.message || 'Failed to delete API key',
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
      try {
        const textarea = document.createElement('textarea');
        textarea.value = text;
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
        setCopiedKey(true);
        setTimeout(() => setCopiedKey(false), 2000);
      } catch {
        setToast({ type: 'error', message: 'Unable to copy to clipboard' });
      }
    }
  };

  // ---------------------------------------------------------------------------
  // Render helpers
  // ---------------------------------------------------------------------------

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'active':
        return (
          <Badge className="flex items-center gap-1 w-fit rounded-md border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 hover:bg-emerald-50">
            <ShieldCheck size={12} />
            Active
          </Badge>
        );
      case 'expired':
        return (
          <Badge className="flex items-center gap-1 w-fit rounded-md border border-amber-200 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700 hover:bg-amber-50">
            <Clock size={12} />
            Expired
          </Badge>
        );
      case 'revoked':
        return (
          <Badge className="flex items-center gap-1 w-fit rounded-md border border-red-200 bg-red-50 px-2 py-0.5 text-xs font-medium text-red-700 hover:bg-red-50">
            <ShieldX size={12} />
            Revoked
          </Badge>
        );
      default:
        return (
          <Badge className="flex items-center gap-1 w-fit rounded-md border border-gray-200 bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 hover:bg-gray-100">
            {status || 'unknown'}
          </Badge>
        );
    }
  };

  const formatDate = (dateStr: string | null) => {
    if (!dateStr) return 'N/A';
    const d = new Date(dateStr);
    if (isNaN(d.getTime())) return 'N/A';
    return d.toLocaleString();
  };

  const isExpired = (expiresAt: string | null) => {
    if (!expiresAt || !nowMs) return false;
    const d = new Date(expiresAt);
    if (isNaN(d.getTime())) return false;
    return d.getTime() < nowMs;
  };

  // ---------------------------------------------------------------------------
  // Loading state
  // ---------------------------------------------------------------------------

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="h-8 w-8 animate-spin text-brand-600" aria-hidden="true" />
        <span className="sr-only">Loading API keys</span>
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
          role="status"
          className={`fixed top-4 right-4 z-50 flex items-center gap-3 rounded-md border px-4 py-3 shadow-lg ${
            toast.type === 'success'
              ? 'bg-emerald-50 border-emerald-200 text-emerald-700'
              : 'bg-red-50 border-red-200 text-red-700'
          }`}
        >
          {toast.type === 'success' ? <CheckCircle size={18} /> : <XCircle size={18} />}
          <span className="text-sm font-medium">{toast.message}</span>
          <button
            type="button"
            onClick={() => setToast(null)}
            aria-label="Dismiss notification"
            className="ml-2 rounded-md p-0.5 hover:bg-white/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            <X size={14} />
          </button>
        </div>
      )}

      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">API Keys</h1>
          <p className="mt-1 text-sm text-gray-600">
            Manage API keys for programmatic access
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            onClick={fetchKeys}
            className="rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
          >
            <RefreshCw size={16} className="mr-2" />
            Refresh
          </Button>
          <Button
            onClick={openCreateForm}
            className="rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700"
          >
            <Plus size={16} className="mr-2" />
            Create API Key
          </Button>
        </div>
      </div>

      {/* Error banner */}
      {error && (
        <div className="flex items-center gap-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-red-700">
          <AlertTriangle size={18} />
          <span className="text-sm">{error}</span>
          <button
            type="button"
            onClick={() => setError(null)}
            aria-label="Dismiss error"
            className="ml-auto rounded-md p-0.5 hover:bg-red-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
          >
            <X size={14} />
          </button>
        </div>
      )}

      {/* Summary Cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card className="border border-gray-200 bg-white shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 px-5 py-4 pb-2">
            <CardTitle className="text-sm font-semibold text-gray-600">Total Keys</CardTitle>
            <Key className="h-4 w-4 text-gray-500" />
          </CardHeader>
          <CardContent className="px-5 pb-5">
            <div className="text-2xl font-semibold text-gray-900">{stats.total}</div>
            <p className="mt-1 text-xs text-gray-500">All API keys</p>
          </CardContent>
        </Card>

        <Card className="border border-gray-200 bg-white shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 px-5 py-4 pb-2">
            <CardTitle className="text-sm font-semibold text-gray-600">Active</CardTitle>
            <ShieldCheck className="h-4 w-4 text-emerald-600" />
          </CardHeader>
          <CardContent className="px-5 pb-5">
            <div className="text-2xl font-semibold text-emerald-600">{stats.active}</div>
            <p className="mt-1 text-xs text-gray-500">Ready to use</p>
          </CardContent>
        </Card>

        <Card className="border border-gray-200 bg-white shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 px-5 py-4 pb-2">
            <CardTitle className="text-sm font-semibold text-gray-600">Expired</CardTitle>
            <Clock className="h-4 w-4 text-amber-600" />
          </CardHeader>
          <CardContent className="px-5 pb-5">
            <div className="text-2xl font-semibold text-amber-600">{stats.expired}</div>
            <p className="mt-1 text-xs text-gray-500">Past expiration date</p>
          </CardContent>
        </Card>

        <Card className="border border-gray-200 bg-white shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 px-5 py-4 pb-2">
            <CardTitle className="text-sm font-semibold text-gray-600">Revoked</CardTitle>
            <ShieldX className="h-4 w-4 text-red-600" />
          </CardHeader>
          <CardContent className="px-5 pb-5">
            <div className="text-2xl font-semibold text-red-600">{stats.revoked}</div>
            <p className="mt-1 text-xs text-gray-500">Access revoked</p>
          </CardContent>
        </Card>
      </div>

      {/* Filters */}
      <Card className="border border-gray-200 bg-white shadow-sm">
        <CardContent className="p-5">
          <div className="flex flex-col gap-4 md:flex-row">
            <div className="relative flex-1">
              <Filter
                className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
                size={16}
                aria-hidden="true"
              />
              <Input
                aria-label="Search API keys"
                placeholder="Search by name, prefix, or scope..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className={`${INPUT_CLASS} pl-10`}
              />
            </div>
            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger
                aria-label="Filter by status"
                className="w-[160px] rounded-md border border-gray-300 bg-white text-sm text-gray-900 focus:ring-1 focus:ring-brand-500 focus:ring-offset-0"
              >
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent className="border border-gray-200 bg-white shadow-lg">
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
      <Card className="border border-gray-200 bg-white shadow-sm">
        <CardHeader className="border-b border-gray-200 px-5 py-4">
          <CardTitle className="flex items-center justify-between text-sm font-semibold text-gray-900">
            <span>API Keys</span>
            <span className="text-sm font-normal text-gray-500">
              {filteredKeys.length} of {safeKeys.length} keys
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {filteredKeys.length === 0 ? (
            <div className="py-16 text-center">
              <Key className="mx-auto text-gray-300" size={48} aria-hidden="true" />
              <h3 className="mt-4 text-sm font-semibold text-gray-900">
                {safeKeys.length === 0 ? 'No API keys' : 'No matching keys'}
              </h3>
              <p className="mt-1 text-sm text-gray-600">
                {safeKeys.length === 0
                  ? 'Create your first API key to get started'
                  : 'Try adjusting your filters or search query'}
              </p>
              {safeKeys.length === 0 && (
                <Button
                  onClick={openCreateForm}
                  className="mt-4 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700"
                >
                  <Plus size={16} className="mr-2" />
                  Create API Key
                </Button>
              )}
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead className="bg-gray-50">
                  <tr className="border-b border-gray-200">
                    <th className={TH_CLASS}>Name</th>
                    <th className={TH_CLASS}>Key Prefix</th>
                    <th className={TH_CLASS}>Scopes</th>
                    <th className={TH_CLASS}>Status</th>
                    <th className={TH_CLASS}>Last Used</th>
                    <th className={TH_CLASS}>Expires At</th>
                    <th className={TH_CLASS}>Created At</th>
                    <th className={`${TH_CLASS} text-right`}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredKeys.map((apiKey) => {
                    const scopes = Array.isArray(apiKey?.scopes) ? apiKey.scopes : [];
                    return (
                      <tr
                        key={apiKey.id}
                        className="border-b border-gray-100 hover:bg-gray-50"
                      >
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-2">
                            <Key size={14} className="text-gray-400" aria-hidden="true" />
                            <span className="text-sm font-medium text-gray-900">
                              {apiKey?.name || '—'}
                            </span>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <span className="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2.5 py-1 font-mono text-sm text-gray-700">
                            {apiKey?.key_prefix || ''}••••••••
                          </span>
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex flex-wrap gap-1">
                            {scopes.length > 0 ? (
                              scopes.map((scope, idx) => (
                                <Badge
                                  key={idx}
                                  className="rounded-md border border-gray-200 bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 hover:bg-gray-100"
                                >
                                  {scope}
                                </Badge>
                              ))
                            ) : (
                              <span className="text-xs text-gray-500">No scopes</span>
                            )}
                          </div>
                        </td>
                        <td className="px-4 py-3">{getStatusBadge(apiKey?.status)}</td>
                        <td className="px-4 py-3 text-sm text-gray-700">
                          {formatDate(apiKey?.last_used ?? null)}
                        </td>
                        <td className="px-4 py-3">
                          <span
                            className={`text-sm ${
                              isExpired(apiKey?.expires_at ?? null)
                                ? 'text-red-600'
                                : apiKey?.expires_at
                                ? 'text-gray-700'
                                : 'text-gray-500'
                            }`}
                          >
                            {apiKey?.expires_at ? formatDate(apiKey.expires_at) : 'Never'}
                          </span>
                        </td>
                        <td className="px-4 py-3 text-sm text-gray-700">
                          {formatDate(apiKey?.created_at ?? null)}
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex items-center justify-end gap-1">
                            {apiKey?.status === 'active' && (
                              <>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => openEditForm(apiKey)}
                                  className="text-gray-600 hover:bg-gray-100 hover:text-gray-900"
                                  title="Edit"
                                  aria-label={`Edit API key ${apiKey?.name || ''}`}
                                >
                                  <Edit size={14} />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => confirmRevoke(apiKey)}
                                  className="text-amber-600 hover:bg-amber-50 hover:text-amber-700"
                                  title="Revoke"
                                  aria-label={`Revoke API key ${apiKey?.name || ''}`}
                                >
                                  <AlertOctagon size={14} />
                                </Button>
                              </>
                            )}
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => confirmDelete(apiKey)}
                              className="text-red-600 hover:bg-red-50 hover:text-red-700"
                              title="Delete"
                              aria-label={`Delete API key ${apiKey?.name || ''}`}
                            >
                              <Trash2 size={14} />
                            </Button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
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
          <div className="absolute inset-0 bg-gray-900/40" onClick={closeForm} />

          {/* Modal */}
          <div
            role="dialog"
            aria-modal="true"
            aria-label={editingKey ? 'Edit API key' : 'Create API key'}
            className="relative mx-4 w-full max-w-lg rounded-lg border border-gray-200 bg-white shadow-lg"
          >
            {/* Header */}
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                <Key size={16} className="text-gray-500" aria-hidden="true" />
                {editingKey ? 'Edit API Key' : 'Create API Key'}
              </h2>
              <button
                type="button"
                onClick={closeForm}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X size={18} />
              </button>
            </div>

            {/* Form */}
            <form onSubmit={handleSubmit} className="space-y-4 px-5 py-4">
              {formError && (
                <div className="flex items-center gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
                  <AlertTriangle size={14} />
                  {formError}
                </div>
              )}

              {/* Name */}
              <div>
                <label htmlFor="api-key-name" className={LABEL_CLASS}>
                  Name <span className="text-red-600">*</span>
                </label>
                <Input
                  id="api-key-name"
                  placeholder="e.g. CI/CD Pipeline, Monitoring Agent"
                  value={formData.name}
                  onChange={(e) => handleFormChange('name', e.target.value)}
                  className={INPUT_CLASS}
                  required
                />
              </div>

              {/* Scopes */}
              <div>
                <label htmlFor="api-key-scopes" className={LABEL_CLASS}>
                  Scopes
                </label>
                <Input
                  id="api-key-scopes"
                  placeholder="e.g. read, write, admin (comma-separated)"
                  value={formData.scopes}
                  onChange={(e) => handleFormChange('scopes', e.target.value)}
                  className={INPUT_CLASS}
                />
                <p className="mt-1 text-xs text-gray-500">
                  Comma-separated list of permission scopes
                </p>
              </div>

              {/* Expiration */}
              <div>
                <label htmlFor="api-key-expires" className={LABEL_CLASS}>
                  Expiration Date
                </label>
                <Input
                  id="api-key-expires"
                  type="date"
                  value={formData.expires_at}
                  onChange={(e) => handleFormChange('expires_at', e.target.value)}
                  className={INPUT_CLASS}
                  min={todayStr || undefined}
                />
                <p className="mt-1 text-xs text-gray-500">Leave empty for no expiration</p>
              </div>

              {/* Actions */}
              <div className="flex items-center justify-end gap-3 border-t border-gray-200 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={closeForm}
                  className="rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={submitting}
                  className="rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-50"
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
          <div className="absolute inset-0 bg-gray-900/40" onClick={() => setCreatedKey(null)} />

          {/* Modal */}
          <div
            role="dialog"
            aria-modal="true"
            aria-label="API key created"
            className="relative mx-4 w-full max-w-lg rounded-lg border border-gray-200 bg-white shadow-lg"
          >
            {/* Header */}
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                <ShieldCheck size={16} className="text-emerald-600" aria-hidden="true" />
                API Key Created
              </h2>
              <button
                type="button"
                onClick={() => setCreatedKey(null)}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X size={18} />
              </button>
            </div>

            <div className="space-y-4 px-5 py-4">
              {/* Warning */}
              <div className="flex items-start gap-3 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-amber-700">
                <AlertTriangle size={18} className="mt-0.5 flex-shrink-0" />
                <div>
                  <p className="text-sm font-medium">Save this key now</p>
                  <p className="mt-1 text-xs text-amber-700">
                    This is the only time the full API key will be displayed. Copy it now and store
                    it securely — it cannot be retrieved again.
                  </p>
                </div>
              </div>

              {/* Key display */}
              <div>
                <span className={LABEL_CLASS}>Your API Key</span>
                <div className="relative">
                  <div className="flex items-center gap-2">
                    <div className="flex-1 select-all break-all rounded-md border border-gray-300 bg-gray-50 px-3 py-2.5 font-mono text-sm text-gray-900">
                      {showCreatedKey ? createdKey : '•'.repeat(Math.min(createdKey.length, 40))}
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => setShowCreatedKey(!showCreatedKey)}
                      className="flex-shrink-0 text-gray-600 hover:bg-gray-100 hover:text-gray-900"
                      title={showCreatedKey ? 'Hide key' : 'Show key'}
                      aria-label={showCreatedKey ? 'Hide API key' : 'Show API key'}
                    >
                      {showCreatedKey ? <EyeOff size={16} /> : <Eye size={16} />}
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => copyToClipboard(createdKey)}
                      className={`flex-shrink-0 hover:bg-gray-100 ${
                        copiedKey ? 'text-emerald-600' : 'text-gray-600 hover:text-gray-900'
                      }`}
                      title="Copy to clipboard"
                      aria-label="Copy API key to clipboard"
                    >
                      {copiedKey ? <CheckCircle size={16} /> : <Copy size={16} />}
                    </Button>
                  </div>
                </div>
                {copiedKey && (
                  <p className="mt-1.5 text-xs text-emerald-600">Copied to clipboard</p>
                )}
              </div>

              {/* Close */}
              <div className="flex justify-end pt-2">
                <Button
                  onClick={() => setCreatedKey(null)}
                  className="rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700"
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
          <div className="absolute inset-0 bg-gray-900/40" onClick={() => setRevokingKey(null)} />
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Revoke API key"
            className="relative mx-4 w-full max-w-md rounded-lg border border-gray-200 bg-white shadow-lg"
          >
            <div className="border-b border-gray-200 px-5 py-4">
              <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                <AlertOctagon size={16} className="text-amber-600" aria-hidden="true" />
                Revoke API Key
              </h2>
            </div>
            <div className="space-y-4 px-5 py-4">
              <p className="text-sm text-gray-600">
                Are you sure you want to revoke the API key{' '}
                <span className="font-semibold text-gray-900">
                  &quot;{revokingKey?.name || ''}&quot;
                </span>
                ? This action will immediately invalidate the key and any applications using it will
                lose access.
              </p>
              <div className="flex items-center gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700">
                <AlertTriangle size={14} />
                This action cannot be undone.
              </div>
              <div className="flex items-center justify-end gap-3 pt-2">
                <Button
                  variant="outline"
                  onClick={() => setRevokingKey(null)}
                  className="rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
                >
                  Cancel
                </Button>
                <Button
                  onClick={handleRevoke}
                  disabled={revoking}
                  className="rounded-md bg-amber-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-amber-700 disabled:opacity-50"
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
          <div className="absolute inset-0 bg-gray-900/40" onClick={() => setDeletingKey(null)} />
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Delete API key"
            className="relative mx-4 w-full max-w-md rounded-lg border border-gray-200 bg-white shadow-lg"
          >
            <div className="border-b border-gray-200 px-5 py-4">
              <h2 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
                <Trash2 size={16} className="text-red-600" aria-hidden="true" />
                Delete API Key
              </h2>
            </div>
            <div className="space-y-4 px-5 py-4">
              <p className="text-sm text-gray-600">
                Are you sure you want to permanently delete the API key{' '}
                <span className="font-semibold text-gray-900">
                  &quot;{deletingKey?.name || ''}&quot;
                </span>
                ? This will remove the key record entirely.
              </p>
              <div className="flex items-center gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
                <AlertTriangle size={14} />
                This action is permanent and cannot be undone.
              </div>
              <div className="flex items-center justify-end gap-3 pt-2">
                <Button
                  variant="outline"
                  onClick={() => setDeletingKey(null)}
                  className="rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
                >
                  Cancel
                </Button>
                <Button
                  onClick={handleDelete}
                  disabled={deleting}
                  className="rounded-md bg-red-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
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
