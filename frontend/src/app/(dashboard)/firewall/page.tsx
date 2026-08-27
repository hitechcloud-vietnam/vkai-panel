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
      setRules(res.data.data || res.data || []);
    } catch (err: any) {
      console.error('Failed to load firewall rules:', err);
      setError(err.response?.data?.message || 'Failed to load firewall rules');
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

  const filteredRules = rules.filter((r) => {
    if (statusFilter !== 'all' && r.status !== statusFilter) return false;
    if (actionFilter !== 'all' && r.action !== actionFilter) return false;
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      return (
        r.protocol.toLowerCase().includes(q) ||
        r.port.toLowerCase().includes(q) ||
        r.source.toLowerCase().includes(q) ||
        r.action.toLowerCase().includes(q) ||
        r.direction.toLowerCase().includes(q)
      );
    }
    return true;
  });

  const stats = {
    total: rules.length,
    active: rules.filter((r) => r.status === 'active').length,
    allow: rules.filter((r) => r.action === 'allow').length,
    deny: rules.filter((r) => r.action === 'deny').length,
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
      server_id: rule.server_id,
      protocol: rule.protocol,
      port: rule.port,
      source: rule.source,
      action: rule.action,
      direction: rule.direction,
      status: rule.status,
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
      const msg = err.response?.data?.message || 'An error occurred while saving the rule';
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
        message: err.response?.data?.message || 'Failed to delete firewall rule',
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
        <Badge variant="success" className="flex items-center gap-1 w-fit">
          <CheckCircle size={12} />
          Allow
        </Badge>
      );
    }
    return (
      <Badge variant="destructive" className="flex items-center gap-1 w-fit">
        <XCircle size={12} />
        Deny
      </Badge>
    );
  };

  const getStatusBadge = (status: string) => {
    if (status === 'active') {
      return (
        <Badge variant="success" className="flex items-center gap-1 w-fit">
          <ShieldCheck size={12} />
          Active
        </Badge>
      );
    }
    return (
      <Badge variant="secondary" className="flex items-center gap-1 w-fit">
        <ShieldX size={12} />
        Inactive
      </Badge>
    );
  };

  const getDirectionIcon = (direction: string) => {
    if (direction === 'in') {
      return <ArrowDown size={14} className="text-blue-400" />;
    }
    return <ArrowUp size={14} className="text-orange-400" />;
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return 'N/A';
    return new Date(dateStr).toLocaleString();
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
            <Flame size={28} className="text-orange-500" />
            Firewall
          </h1>
          <p className="text-dark-400 mt-1">
            Manage firewall rules and network security
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Button variant="outline" onClick={fetchRules} className="border-dark-600 text-dark-300 hover:bg-dark-700">
            <RefreshCw size={16} className="mr-2" />
            Refresh
          </Button>
          <Button onClick={openCreateForm} className="bg-primary-600 hover:bg-primary-700 text-white">
            <Plus size={16} className="mr-2" />
            Add Rule
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
            <CardTitle className="text-sm font-medium text-dark-300">Total Rules</CardTitle>
            <Network className="h-4 w-4 text-dark-400" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-dark-50">{stats.total}</div>
            <p className="text-xs text-dark-500 mt-1">All firewall rules</p>
          </CardContent>
        </Card>

        <Card className="bg-dark-800 border-dark-700">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-dark-300">Active Rules</CardTitle>
            <ShieldCheck className="h-4 w-4 text-green-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-400">{stats.active}</div>
            <p className="text-xs text-dark-500 mt-1">Currently enforced</p>
          </CardContent>
        </Card>

        <Card className="bg-dark-800 border-dark-700">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-dark-300">Allow Rules</CardTitle>
            <CheckCircle className="h-4 w-4 text-blue-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-blue-400">{stats.allow}</div>
            <p className="text-xs text-dark-500 mt-1">Permitted traffic</p>
          </CardContent>
        </Card>

        <Card className="bg-dark-800 border-dark-700">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium text-dark-300">Deny Rules</CardTitle>
            <XCircle className="h-4 w-4 text-red-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-red-400">{stats.deny}</div>
            <p className="text-xs text-dark-500 mt-1">Blocked traffic</p>
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
                placeholder="Search by protocol, port, source..."
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
                <SelectItem value="inactive">Inactive</SelectItem>
              </SelectContent>
            </Select>
            <Select value={actionFilter} onValueChange={setActionFilter}>
              <SelectTrigger className="w-[160px] bg-dark-900 border-dark-600 text-dark-200">
                <SelectValue placeholder="Action" />
              </SelectTrigger>
              <SelectContent className="bg-dark-800 border-dark-600">
                <SelectItem value="all">All Actions</SelectItem>
                <SelectItem value="allow">Allow</SelectItem>
                <SelectItem value="deny">Deny</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* Rules Table */}
      <Card className="bg-dark-800 border-dark-700">
        <CardHeader>
          <CardTitle className="text-dark-100 flex items-center justify-between">
            <span>Firewall Rules</span>
            <span className="text-sm font-normal text-dark-400">
              {filteredRules.length} of {rules.length} rules
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {filteredRules.length === 0 ? (
            <div className="text-center py-16">
              <Flame className="mx-auto text-dark-600" size={64} />
              <h3 className="mt-4 text-xl font-medium text-dark-300">
                {rules.length === 0 ? 'No firewall rules' : 'No matching rules'}
              </h3>
              <p className="mt-2 text-dark-500">
                {rules.length === 0
                  ? 'Add your first firewall rule to get started'
                  : 'Try adjusting your filters or search query'}
              </p>
              {rules.length === 0 && (
                <Button
                  onClick={openCreateForm}
                  className="mt-4 bg-primary-600 hover:bg-primary-700 text-white"
                >
                  <Plus size={16} className="mr-2" />
                  Add Rule
                </Button>
              )}
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-dark-700">
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Protocol</th>
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Port</th>
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Source</th>
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Action</th>
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Direction</th>
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Status</th>
                    <th className="text-left p-3 text-sm font-medium text-dark-400">Created At</th>
                    <th className="text-right p-3 text-sm font-medium text-dark-400">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredRules.map((rule) => (
                    <tr
                      key={rule.id}
                      className="border-b border-dark-700/50 hover:bg-dark-700/30 transition-colors"
                    >
                      <td className="p-3">
                        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md bg-dark-700 text-dark-100 text-sm font-mono uppercase">
                          {rule.protocol}
                        </span>
                      </td>
                      <td className="p-3 text-sm text-dark-200 font-mono">
                        {rule.port || 'Any'}
                      </td>
                      <td className="p-3 text-sm text-dark-200 font-mono">
                        {rule.source || '0.0.0.0/0'}
                      </td>
                      <td className="p-3">{getActionBadge(rule.action)}</td>
                      <td className="p-3">
                        <div className="flex items-center gap-1.5 text-sm text-dark-200">
                          {getDirectionIcon(rule.direction)}
                          <span className="uppercase">{rule.direction}</span>
                        </div>
                      </td>
                      <td className="p-3">{getStatusBadge(rule.status)}</td>
                      <td className="p-3 text-sm text-dark-400">
                        {formatDate(rule.created_at)}
                      </td>
                      <td className="p-3">
                        <div className="flex items-center justify-end gap-2">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => openEditForm(rule)}
                            className="text-dark-300 hover:text-dark-100 hover:bg-dark-700"
                          >
                            <Edit size={14} />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => confirmDelete(rule)}
                            className="text-red-400 hover:text-red-300 hover:bg-red-900/20"
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
                <Shield size={20} className="text-primary-400" />
                {editingRule ? 'Edit Firewall Rule' : 'Add Firewall Rule'}
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

              {/* Server ID */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1.5">
                  Server ID <span className="text-red-400">*</span>
                </label>
                <Input
                  placeholder="Enter server UUID"
                  value={formData.server_id}
                  onChange={(e) => handleFormChange('server_id', e.target.value)}
                  className="bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
                  required
                />
              </div>

              {/* Protocol & Port */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1.5">
                    Protocol <span className="text-red-400">*</span>
                  </label>
                  <Select
                    value={formData.protocol}
                    onValueChange={(v) => handleFormChange('protocol', v)}
                  >
                    <SelectTrigger className="bg-dark-900 border-dark-600 text-dark-200">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="bg-dark-800 border-dark-600">
                      <SelectItem value="tcp">TCP</SelectItem>
                      <SelectItem value="udp">UDP</SelectItem>
                      <SelectItem value="icmp">ICMP</SelectItem>
                      <SelectItem value="tcp+udp">TCP+UDP</SelectItem>
                      <SelectItem value="any">Any</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1.5">
                    Port
                  </label>
                  <Input
                    placeholder="e.g. 80, 443, 8000-9000"
                    value={formData.port}
                    onChange={(e) => handleFormChange('port', e.target.value)}
                    className="bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
                  />
                </div>
              </div>

              {/* Source */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1.5">
                  Source
                </label>
                <Input
                  placeholder="e.g. 0.0.0.0/0, 192.168.1.0/24"
                  value={formData.source}
                  onChange={(e) => handleFormChange('source', e.target.value)}
                  className="bg-dark-900 border-dark-600 text-dark-100 placeholder-dark-500 focus:border-primary-500"
                />
              </div>

              {/* Action & Direction */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1.5">
                    Action <span className="text-red-400">*</span>
                  </label>
                  <Select
                    value={formData.action}
                    onValueChange={(v: 'allow' | 'deny') => handleFormChange('action', v)}
                  >
                    <SelectTrigger className="bg-dark-900 border-dark-600 text-dark-200">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="bg-dark-800 border-dark-600">
                      <SelectItem value="allow">Allow</SelectItem>
                      <SelectItem value="deny">Deny</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <label className="block text-sm font-medium text-dark-300 mb-1.5">
                    Direction
                  </label>
                  <Select
                    value={formData.direction}
                    onValueChange={(v: 'in' | 'out') => handleFormChange('direction', v)}
                  >
                    <SelectTrigger className="bg-dark-900 border-dark-600 text-dark-200">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="bg-dark-800 border-dark-600">
                      <SelectItem value="in">Inbound</SelectItem>
                      <SelectItem value="out">Outbound</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              {/* Status */}
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-1.5">
                  Status
                </label>
                <Select
                  value={formData.status}
                  onValueChange={(v) => handleFormChange('status', v)}
                >
                  <SelectTrigger className="bg-dark-900 border-dark-600 text-dark-200">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className="bg-dark-800 border-dark-600">
                    <SelectItem value="active">Active</SelectItem>
                    <SelectItem value="inactive">Inactive</SelectItem>
                  </SelectContent>
                </Select>
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
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => !deleting && setDeletingRule(null)}
          />

          {/* Modal */}
          <div className="relative w-full max-w-md mx-4 bg-dark-800 border border-dark-600 rounded-xl shadow-2xl">
            <div className="px-6 py-5">
              <div className="flex items-center gap-3 mb-4">
                <div className="p-2 bg-red-900/30 rounded-lg">
                  <AlertTriangle size={24} className="text-red-400" />
                </div>
                <div>
                  <h3 className="text-lg font-semibold text-dark-50">Delete Rule</h3>
                  <p className="text-sm text-dark-400">This action cannot be undone</p>
                </div>
              </div>

              <div className="mb-5 p-3 rounded-lg bg-dark-900 border border-dark-700">
                <p className="text-sm text-dark-300">
                  Are you sure you want to delete this{' '}
                  <span className="font-mono uppercase font-semibold text-dark-100">
                    {deletingRule.protocol}
                  </span>{' '}
                  rule on port{' '}
                  <span className="font-mono font-semibold text-dark-100">
                    {deletingRule.port || 'any'}
                  </span>
                  ?
                </p>
              </div>

              <div className="flex items-center justify-end gap-3">
                <Button
                  variant="outline"
                  onClick={() => setDeletingRule(null)}
                  disabled={deleting}
                  className="border-dark-600 text-dark-300 hover:bg-dark-700"
                >
                  Cancel
                </Button>
                <Button
                  variant="destructive"
                  onClick={handleDelete}
                  disabled={deleting}
                  className="bg-red-600 hover:bg-red-700 text-white disabled:opacity-50"
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
