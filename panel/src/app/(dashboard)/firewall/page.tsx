'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  Flame,
  Plus,
  Trash2,
  Edit,
  Shield,
  ShieldCheck,
  ShieldX,
  Filter,
  RefreshCw,
  ArrowUp,
  ArrowDown,
  X,
  AlertTriangle,
  CheckCircle,
  XCircle,
  Network,
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
import { firewallApi } from '@/services/api';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface FirewallRule {
  id: string;
  tenant_id: string;
  server_id: string;
  protocol: string;
  port: string;
  source: string;
  action: 'allow' | 'deny';
  direction: 'in' | 'out';
  status: string;
  created_at: string;
  updated_at: string;
}

interface RuleFormData {
  server_id: string;
  protocol: string;
  port: string;
  source: string;
  action: 'allow' | 'deny';
  direction: 'in' | 'out';
  status: string;
}

const EMPTY_FORM: RuleFormData = {
  server_id: '',
  protocol: 'tcp',
  port: '',
  source: '',
  action: 'allow',
  direction: 'in',
  status: 'active',
};

// Shared control classes (light enterprise)
const INPUT_CLASS =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus-visible:border-blue-500 focus-visible:ring-1 focus-visible:ring-blue-500 focus-visible:ring-offset-0 focus-visible:outline-none';
const SELECT_TRIGGER_CLASS =
  'rounded-md border border-gray-300 bg-white text-sm text-gray-900 focus:border-blue-500 focus:ring-1 focus:ring-blue-500';
const SELECT_CONTENT_CLASS = 'bg-white border border-gray-200 shadow-lg';
const SELECT_ITEM_CLASS = 'text-gray-900 focus:bg-gray-100 focus:text-gray-900';
const BTN_PRIMARY =
  'bg-blue-600 text-white hover:bg-blue-700 rounded-md text-sm font-medium';
const BTN_SECONDARY =
  'bg-white border border-gray-300 text-gray-700 hover:bg-gray-50 rounded-md text-sm font-medium';
const BTN_DANGER =
  'bg-red-600 text-white hover:bg-red-700 rounded-md text-sm font-medium disabled:opacity-50';

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function FirewallPage() {
  // Data
  const [rules, setRules] = useState<FirewallRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filters
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [actionFilter, setActionFilter] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState('');

  // Modal state
  const [showForm, setShowForm] = useState(false);
  const [editingRule, setEditingRule] = useState<FirewallRule | null>(null);
  const [formData, setFormData] = useState<RuleFormData>(EMPTY_FORM);
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  // Delete confirmation
  const [deletingRule, setDeletingRule] = useState<FirewallRule | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Toast
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // ---------------------------------------------------------------------------
  // Data fetching
  // ---------------------------------------------------------------------------

  const fetchRules = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await firewallApi.list();
      const payload = Array.isArray(res?.data?.data)
        ? res.data.data
        : Array.isArray(res?.data)
        ? res.data
        : [];
      setRules(payload);
    } catch (err: any) {
      console.error('Failed to load firewall rules:', err);
      setRules([]);
      setError(err?.response?.data?.message || 'Failed to load firewall rules');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchRules();
  }, [fetchRules]);

  // Auto-dismiss toast
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 4000);
    return () => clearTimeout(t);
  }, [toast]);

  // ---------------------------------------------------------------------------
  // Computed / filtered data
  // ---------------------------------------------------------------------------

  const safeRules = Array.isArray(rules) ? rules : [];

  const filteredRules = safeRules.filter((r) => {
    if (!r) return false;
    if (statusFilter !== 'all' && r.status !== statusFilter) return false;
    if (actionFilter !== 'all' && r.action !== actionFilter) return false;
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      return (
        (r.protocol ?? '').toLowerCase().includes(q) ||
        (r.port ?? '').toLowerCase().includes(q) ||
        (r.source ?? '').toLowerCase().includes(q) ||
        (r.action ?? '').toLowerCase().includes(q) ||
        (r.direction ?? '').toLowerCase().includes(q)
      );
    }
    return true;
  });

  const stats = {
    total: safeRules.length,
    active: safeRules.filter((r) => r?.status === 'active').length,
    allow: safeRules.filter((r) => r?.action === 'allow').length,
    deny: safeRules.filter((r) => r?.action === 'deny').length,
  };

  // ---------------------------------------------------------------------------
  // Form helpers
  // ---------------------------------------------------------------------------

  const openCreateForm = () => {
    setEditingRule(null);
    setFormData(EMPTY_FORM);
    setFormError(null);
    setShowForm(true);
  };

  const openEditForm = (rule: FirewallRule) => {
    setEditingRule(rule);
    setFormData({
      server_id: rule?.server_id ?? '',
      protocol: rule?.protocol ?? 'tcp',
      port: rule?.port ?? '',
      source: rule?.source ?? '',
      action: rule?.action ?? 'allow',
      direction: rule?.direction ?? 'in',
      status: rule?.status ?? 'active',
    });
    setFormError(null);
    setShowForm(true);
  };

  const closeForm = () => {
    setShowForm(false);
    setEditingRule(null);
    setFormError(null);
  };

  const handleFormChange = (field: keyof RuleFormData, value: string) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);

    // Basic validation
    if (!formData.protocol) {
      setFormError('Protocol is required');
      return;
    }
    if (!formData.action) {
      setFormError('Action is required');
      return;
    }

    try {
      setSubmitting(true);
      if (editingRule) {
        await firewallApi.update(editingRule.id, formData);
        setToast({ type: 'success', message: 'Firewall rule updated successfully' });
      } else {
        await firewallApi.create(formData);
        setToast({ type: 'success', message: 'Firewall rule created successfully' });
      }
      closeForm();
      fetchRules();
    } catch (err: any) {
      const msg = err?.response?.data?.message || 'An error occurred while saving the rule';
      setFormError(msg);
    } finally {
      setSubmitting(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Delete helpers
  // ---------------------------------------------------------------------------

  const confirmDelete = (rule: FirewallRule) => {
    setDeletingRule(rule);
  };

  const handleDelete = async () => {
    if (!deletingRule) return;
    try {
      setDeleting(true);
      await firewallApi.delete(deletingRule.id);
      setToast({ type: 'success', message: 'Firewall rule deleted successfully' });
      setDeletingRule(null);
      fetchRules();
    } catch (err: any) {
      setToast({
        type: 'error',
        message: err?.response?.data?.message || 'Failed to delete firewall rule',
      });
    } finally {
      setDeleting(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Render helpers
  // ---------------------------------------------------------------------------

  const getActionBadge = (action: string) => {
    if (action === 'allow') {
      return (
        <Badge
          variant="success"
          className="flex items-center gap-1 w-fit rounded-md border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 hover:bg-emerald-50"
        >
          <CheckCircle size={12} />
          Allow
        </Badge>
      );
    }
    return (
      <Badge
        variant="destructive"
        className="flex items-center gap-1 w-fit rounded-md border border-red-200 bg-red-50 px-2 py-0.5 text-xs font-medium text-red-700 hover:bg-red-50"
      >
        <XCircle size={12} />
        Deny
      </Badge>
    );
  };

  const getStatusBadge = (status: string) => {
    if (status === 'active') {
      return (
        <Badge
          variant="success"
          className="flex items-center gap-1 w-fit rounded-md border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 hover:bg-emerald-50"
        >
          <ShieldCheck size={12} />
          Active
        </Badge>
      );
    }
    return (
      <Badge
        variant="secondary"
        className="flex items-center gap-1 w-fit rounded-md border border-gray-200 bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 hover:bg-gray-100"
      >
        <ShieldX size={12} />
        Inactive
      </Badge>
    );
  };

  const getDirectionIcon = (direction: string) => {
    if (direction === 'in') {
      return <ArrowDown size={14} className="text-blue-600" />;
    }
    return <ArrowUp size={14} className="text-amber-600" />;
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return 'N/A';
    const d = new Date(dateStr);
    if (Number.isNaN(d.getTime())) return 'N/A';
    return d.toLocaleString();
  };

  // ---------------------------------------------------------------------------
  // Main render
  // ---------------------------------------------------------------------------

  return (
    <div className="space-y-6">
      {/* Toast */}
      {toast && (
        <div
          role="status"
          className={`fixed top-4 right-4 z-50 flex items-center gap-3 px-4 py-3 rounded-md shadow-lg border ${
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
            className="ml-2 rounded-md p-1 hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
          >
            <X size={14} />
          </button>
        </div>
      )}

      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900 flex items-center gap-2">
            <Flame size={20} className="text-gray-500" />
            Firewall
          </h1>
          <p className="text-sm text-gray-600 mt-1">
            Manage firewall rules and network security
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Button variant="outline" onClick={fetchRules} className={BTN_SECONDARY}>
            <RefreshCw size={16} className="mr-2" />
            Refresh
          </Button>
          <Button onClick={openCreateForm} className={BTN_PRIMARY}>
            <Plus size={16} className="mr-2" />
            Add Rule
          </Button>
        </div>
      </div>

      {/* Error banner */}
      {error && (
        <div className="flex items-center gap-3 px-4 py-3 rounded-lg bg-red-50 border border-red-200 text-red-700">
          <AlertTriangle size={18} />
          <span className="text-sm">{error}</span>
          <button
            type="button"
            onClick={() => setError(null)}
            aria-label="Dismiss error"
            className="ml-auto rounded-md p-1 hover:bg-red-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
          >
            <X size={14} />
          </button>
        </div>
      )}

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 px-5 pt-5 pb-2">
            <CardTitle className="text-sm font-medium text-gray-500">Total Rules</CardTitle>
            <Network className="h-4 w-4 text-gray-400" />
          </CardHeader>
          <CardContent className="px-5 pb-5">
            <div className="text-2xl font-semibold text-gray-900">{stats.total}</div>
            <p className="text-xs text-gray-500 mt-1">All firewall rules</p>
          </CardContent>
        </Card>

        <Card className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 px-5 pt-5 pb-2">
            <CardTitle className="text-sm font-medium text-gray-500">Active Rules</CardTitle>
            <ShieldCheck className="h-4 w-4 text-emerald-600" />
          </CardHeader>
          <CardContent className="px-5 pb-5">
            <div className="text-2xl font-semibold text-gray-900">{stats.active}</div>
            <p className="text-xs text-gray-500 mt-1">Currently enforced</p>
          </CardContent>
        </Card>

        <Card className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 px-5 pt-5 pb-2">
            <CardTitle className="text-sm font-medium text-gray-500">Allow Rules</CardTitle>
            <CheckCircle className="h-4 w-4 text-blue-600" />
          </CardHeader>
          <CardContent className="px-5 pb-5">
            <div className="text-2xl font-semibold text-gray-900">{stats.allow}</div>
            <p className="text-xs text-gray-500 mt-1">Permitted traffic</p>
          </CardContent>
        </Card>

        <Card className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 px-5 pt-5 pb-2">
            <CardTitle className="text-sm font-medium text-gray-500">Deny Rules</CardTitle>
            <XCircle className="h-4 w-4 text-red-600" />
          </CardHeader>
          <CardContent className="px-5 pb-5">
            <div className="text-2xl font-semibold text-gray-900">{stats.deny}</div>
            <p className="text-xs text-gray-500 mt-1">Blocked traffic</p>
          </CardContent>
        </Card>
      </div>

      {/* Filters */}
      <Card className="bg-white border border-gray-200 rounded-lg shadow-sm">
        <CardContent className="p-4">
          <div className="flex flex-col md:flex-row gap-3">
            <div className="flex-1 relative">
              <Filter className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" size={16} />
              <Input
                placeholder="Search by protocol, port, source..."
                aria-label="Search firewall rules"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className={`pl-10 ${INPUT_CLASS}`}
              />
            </div>
            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger aria-label="Filter by status" className={`w-full md:w-[160px] ${SELECT_TRIGGER_CLASS}`}>
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent className={SELECT_CONTENT_CLASS}>
                <SelectItem value="all" className={SELECT_ITEM_CLASS}>All Statuses</SelectItem>
                <SelectItem value="active" className={SELECT_ITEM_CLASS}>Active</SelectItem>
                <SelectItem value="inactive" className={SELECT_ITEM_CLASS}>Inactive</SelectItem>
              </SelectContent>
            </Select>
            <Select value={actionFilter} onValueChange={setActionFilter}>
              <SelectTrigger aria-label="Filter by action" className={`w-full md:w-[160px] ${SELECT_TRIGGER_CLASS}`}>
                <SelectValue placeholder="Action" />
              </SelectTrigger>
              <SelectContent className={SELECT_CONTENT_CLASS}>
                <SelectItem value="all" className={SELECT_ITEM_CLASS}>All Actions</SelectItem>
                <SelectItem value="allow" className={SELECT_ITEM_CLASS}>Allow</SelectItem>
                <SelectItem value="deny" className={SELECT_ITEM_CLASS}>Deny</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* Rules Table */}
      <Card className="bg-white border border-gray-200 rounded-lg shadow-sm overflow-hidden">
        <CardHeader className="px-5 py-4 border-b border-gray-200">
          <CardTitle className="text-sm font-semibold text-gray-900 flex items-center justify-between">
            <span>Firewall Rules</span>
            <span className="text-sm font-normal text-gray-500">
              {filteredRules.length} of {safeRules.length} rules
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="flex items-center justify-center py-20">
              <RefreshCw className="h-8 w-8 animate-spin text-blue-600" />
            </div>
          ) : filteredRules.length === 0 ? (
            <div className="text-center py-16 px-5">
              <Flame className="mx-auto text-gray-400" size={40} />
              <h3 className="mt-4 text-sm font-semibold text-gray-900">
                {safeRules.length === 0 ? 'No firewall rules' : 'No matching rules'}
              </h3>
              <p className="mt-2 text-sm text-gray-600">
                {safeRules.length === 0
                  ? 'Add your first firewall rule to get started'
                  : 'Try adjusting your filters or search query'}
              </p>
              {safeRules.length === 0 && (
                <Button onClick={openCreateForm} className={`mt-6 ${BTN_PRIMARY}`}>
                  <Plus size={16} className="mr-2" />
                  Add Rule
                </Button>
              )}
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead className="bg-gray-50">
                  <tr className="border-b border-gray-200">
                    <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">Protocol</th>
                    <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">Port</th>
                    <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">Source</th>
                    <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">Action</th>
                    <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">Direction</th>
                    <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">Status</th>
                    <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">Created At</th>
                    <th className="text-right px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredRules.map((rule) => (
                    <tr
                      key={rule.id}
                      className="border-b border-gray-100 hover:bg-gray-50 transition-colors"
                    >
                      <td className="px-4 py-3">
                        <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md bg-gray-100 border border-gray-200 text-gray-700 text-xs font-medium font-mono uppercase">
                          {rule.protocol}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-sm text-gray-700 font-mono">
                        {rule.port || 'Any'}
                      </td>
                      <td className="px-4 py-3 text-sm text-gray-700 font-mono">
                        {rule.source || '0.0.0.0/0'}
                      </td>
                      <td className="px-4 py-3">{getActionBadge(rule.action)}</td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1.5 text-sm text-gray-700">
                          {getDirectionIcon(rule.direction)}
                          <span className="uppercase">{rule.direction}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3">{getStatusBadge(rule.status)}</td>
                      <td className="px-4 py-3 text-sm text-gray-600" suppressHydrationWarning>
                        {formatDate(rule.created_at)}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-2">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => openEditForm(rule)}
                            aria-label={`Edit ${rule.protocol} rule on port ${rule.port || 'any'}`}
                            title="Edit rule"
                            className="text-gray-500 hover:text-blue-700 hover:bg-blue-50"
                          >
                            <Edit size={14} />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => confirmDelete(rule)}
                            aria-label={`Delete ${rule.protocol} rule on port ${rule.port || 'any'}`}
                            title="Delete rule"
                            className="text-gray-500 hover:text-red-700 hover:bg-red-50"
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
            className="absolute inset-0 bg-gray-900/50"
            onClick={closeForm}
          />

          {/* Modal */}
          <div
            role="dialog"
            aria-modal="true"
            aria-label={editingRule ? 'Edit Firewall Rule' : 'Add Firewall Rule'}
            className="relative w-full max-w-lg mx-4 bg-white border border-gray-200 rounded-lg shadow-lg"
          >
            {/* Header */}
            <div className="flex items-center justify-between px-5 py-4 border-b border-gray-200">
              <h2 className="text-sm font-semibold text-gray-900 flex items-center gap-2">
                <Shield size={16} className="text-gray-500" />
                {editingRule ? 'Edit Firewall Rule' : 'Add Firewall Rule'}
              </h2>
              <button
                type="button"
                onClick={closeForm}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
              >
                <X size={18} />
              </button>
            </div>

            {/* Form */}
            <form onSubmit={handleSubmit} className="px-5 py-4 space-y-4">
              {formError && (
                <div className="flex items-center gap-2 px-3 py-2 rounded-md bg-red-50 border border-red-200 text-red-700 text-sm">
                  <AlertTriangle size={14} />
                  {formError}
                </div>
              )}

              {/* Server ID */}
              <div>
                <label htmlFor="fw-server-id" className="block text-sm font-medium text-gray-700 mb-1.5">
                  Server ID <span className="text-red-600">*</span>
                </label>
                <Input
                  id="fw-server-id"
                  placeholder="Enter server UUID"
                  value={formData.server_id}
                  onChange={(e) => handleFormChange('server_id', e.target.value)}
                  className={INPUT_CLASS}
                  required
                />
              </div>

              {/* Protocol & Port */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label htmlFor="fw-protocol" className="block text-sm font-medium text-gray-700 mb-1.5">
                    Protocol <span className="text-red-600">*</span>
                  </label>
                  <Select
                    value={formData.protocol}
                    onValueChange={(v) => handleFormChange('protocol', v)}
                  >
                    <SelectTrigger id="fw-protocol" aria-label="Protocol" className={SELECT_TRIGGER_CLASS}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className={SELECT_CONTENT_CLASS}>
                      <SelectItem value="tcp" className={SELECT_ITEM_CLASS}>TCP</SelectItem>
                      <SelectItem value="udp" className={SELECT_ITEM_CLASS}>UDP</SelectItem>
                      <SelectItem value="icmp" className={SELECT_ITEM_CLASS}>ICMP</SelectItem>
                      <SelectItem value="tcp+udp" className={SELECT_ITEM_CLASS}>TCP+UDP</SelectItem>
                      <SelectItem value="any" className={SELECT_ITEM_CLASS}>Any</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <label htmlFor="fw-port" className="block text-sm font-medium text-gray-700 mb-1.5">
                    Port
                  </label>
                  <Input
                    id="fw-port"
                    placeholder="e.g. 80, 443, 8000-9000"
                    value={formData.port}
                    onChange={(e) => handleFormChange('port', e.target.value)}
                    className={INPUT_CLASS}
                  />
                </div>
              </div>

              {/* Source */}
              <div>
                <label htmlFor="fw-source" className="block text-sm font-medium text-gray-700 mb-1.5">
                  Source
                </label>
                <Input
                  id="fw-source"
                  placeholder="e.g. 0.0.0.0/0, 192.168.1.0/24"
                  value={formData.source}
                  onChange={(e) => handleFormChange('source', e.target.value)}
                  className={INPUT_CLASS}
                />
              </div>

              {/* Action & Direction */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label htmlFor="fw-action" className="block text-sm font-medium text-gray-700 mb-1.5">
                    Action <span className="text-red-600">*</span>
                  </label>
                  <Select
                    value={formData.action}
                    onValueChange={(v: 'allow' | 'deny') => handleFormChange('action', v)}
                  >
                    <SelectTrigger id="fw-action" aria-label="Action" className={SELECT_TRIGGER_CLASS}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className={SELECT_CONTENT_CLASS}>
                      <SelectItem value="allow" className={SELECT_ITEM_CLASS}>Allow</SelectItem>
                      <SelectItem value="deny" className={SELECT_ITEM_CLASS}>Deny</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <label htmlFor="fw-direction" className="block text-sm font-medium text-gray-700 mb-1.5">
                    Direction
                  </label>
                  <Select
                    value={formData.direction}
                    onValueChange={(v: 'in' | 'out') => handleFormChange('direction', v)}
                  >
                    <SelectTrigger id="fw-direction" aria-label="Direction" className={SELECT_TRIGGER_CLASS}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className={SELECT_CONTENT_CLASS}>
                      <SelectItem value="in" className={SELECT_ITEM_CLASS}>Inbound</SelectItem>
                      <SelectItem value="out" className={SELECT_ITEM_CLASS}>Outbound</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              {/* Status */}
              <div>
                <label htmlFor="fw-status" className="block text-sm font-medium text-gray-700 mb-1.5">
                  Status
                </label>
                <Select
                  value={formData.status}
                  onValueChange={(v) => handleFormChange('status', v)}
                >
                  <SelectTrigger id="fw-status" aria-label="Status" className={SELECT_TRIGGER_CLASS}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className={SELECT_CONTENT_CLASS}>
                    <SelectItem value="active" className={SELECT_ITEM_CLASS}>Active</SelectItem>
                    <SelectItem value="inactive" className={SELECT_ITEM_CLASS}>Inactive</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {/* Actions */}
              <div className="flex items-center justify-end gap-3 pt-4 border-t border-gray-200">
                <Button
                  type="button"
                  variant="outline"
                  onClick={closeForm}
                  className={BTN_SECONDARY}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  disabled={submitting}
                  className={`${BTN_PRIMARY} disabled:opacity-50`}
                >
                  {submitting ? (
                    <>
                      <RefreshCw size={14} className="mr-2 animate-spin" />
                      Saving...
                    </>
                  ) : editingRule ? (
                    'Update Rule'
                  ) : (
                    'Create Rule'
                  )}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ------------------------------------------------------------------ */}
      {/* Delete Confirmation Modal                                          */}
      {/* ------------------------------------------------------------------ */}
      {deletingRule && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          {/* Backdrop */}
          <div
            className="absolute inset-0 bg-gray-900/50"
            onClick={() => !deleting && setDeletingRule(null)}
          />

          {/* Modal */}
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Delete Rule"
            className="relative w-full max-w-md mx-4 bg-white border border-gray-200 rounded-lg shadow-lg"
          >
            <div className="px-5 py-5">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-red-50 border border-red-200 rounded-md">
                  <AlertTriangle size={20} className="text-red-600" />
                </div>
                <div>
                  <h3 className="text-sm font-semibold text-gray-900">Delete Rule</h3>
                  <p className="text-sm text-gray-600">This action cannot be undone</p>
                </div>
              </div>

              <div className="mb-5 p-3 rounded-md bg-gray-50 border border-gray-200">
                <p className="text-sm text-gray-700">
                  Are you sure you want to delete this{' '}
                  <span className="font-mono uppercase font-semibold text-gray-900">
                    {deletingRule?.protocol ?? ''}
                  </span>{' '}
                  rule on port{' '}
                  <span className="font-mono font-semibold text-gray-900">
                    {deletingRule?.port || 'any'}
                  </span>
                  ?
                </p>
              </div>

              <div className="flex items-center justify-end gap-3">
                <Button
                  variant="outline"
                  onClick={() => setDeletingRule(null)}
                  disabled={deleting}
                  className={BTN_SECONDARY}
                >
                  Cancel
                </Button>
                <Button
                  variant="destructive"
                  onClick={handleDelete}
                  disabled={deleting}
                  className={BTN_DANGER}
                >
                  {deleting ? (
                    <>
                      <RefreshCw size={14} className="mr-2 animate-spin" />
                      Deleting...
                    </>
                  ) : (
                    <>
                      <Trash2 size={14} className="mr-2" />
                      Delete Rule
                    </>
                  )}
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
