'use client';

import { useEffect, useState, useCallback } from 'react';
import {
  Clock,
  Plus,
  Trash2,
  Edit,
  Play,
  Pause,
  Search,
  Filter,
  RefreshCw,
  Terminal,
  Globe,
  Code,
  FileCode,
  AlertCircle,
  CheckCircle2,
  XCircle,
  Loader2,
  X,
  CalendarClock,
  Activity,
  Timer,
} from 'lucide-react';
import { cronApi, api } from '@/services/api';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface CronJob {
  id: string;
  tenant_id: string;
  server_id: string;
  name: string;
  command: string;
  schedule: string;
  type: string; // shell | php | url | nodejs
  status: string; // active | inactive
  last_run_at: string | null;
  next_run_at: string | null;
  created_at: string;
  updated_at: string;
}

interface CronJobFormData {
  name: string;
  command: string;
  schedule: string;
  type: string;
  server_id: string;
}

const CRON_TYPES = ['shell', 'php', 'url', 'nodejs'] as const;
const CRON_STATUSES = ['active', 'inactive'] as const;

const DEFAULT_FORM: CronJobFormData = {
  name: '',
  command: '',
  schedule: '',
  type: 'shell',
  server_id: '',
};

// Shared control classes (light enterprise)
const INPUT_CLASS =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus-visible:border-brand-500 focus-visible:ring-1 focus-visible:ring-brand-500 focus-visible:ring-offset-0 focus-visible:outline-none';
const SELECT_TRIGGER_CLASS =
  'rounded-md border border-gray-300 bg-white text-sm text-gray-900 focus:border-brand-500 focus:ring-1 focus:ring-brand-500';
const SELECT_CONTENT_CLASS = 'bg-white border border-gray-200 shadow-lg';
const SELECT_ITEM_CLASS = 'text-gray-900 focus:bg-gray-100 focus:text-gray-900';
const BTN_PRIMARY =
  'bg-brand-600 text-white hover:bg-brand-700 rounded-md text-sm font-medium';
const BTN_SECONDARY =
  'bg-white border border-gray-300 text-gray-700 hover:bg-gray-50 rounded-md text-sm font-medium';
const BTN_DANGER =
  'bg-red-600 text-white hover:bg-red-700 rounded-md text-sm font-medium';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatDate(iso: string | null): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function relativeTime(iso: string | null): string {
  if (!iso) return 'Never';
  const parsed = new Date(iso).getTime();
  if (Number.isNaN(parsed)) return 'Never';
  const diff = Date.now() - parsed;
  const mins = Math.floor(diff / 60_000);
  if (mins < 1) return 'Just now';
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  return `${days}d ago`;
}

function typeIcon(type: string) {
  switch (type) {
    case 'shell':
      return <Terminal className="h-4 w-4" />;
    case 'php':
      return <Code className="h-4 w-4" />;
    case 'url':
      return <Globe className="h-4 w-4" />;
    case 'nodejs':
      return <FileCode className="h-4 w-4" />;
    default:
      return <Terminal className="h-4 w-4" />;
  }
}

function typeBadgeColor(type: string) {
  switch (type) {
    case 'shell':
      return 'bg-gray-100 text-gray-700 border-gray-200';
    case 'php':
      return 'bg-sky-50 text-sky-700 border-sky-200';
    case 'url':
      return 'bg-brand-50 text-brand-700 border-brand-200';
    case 'nodejs':
      return 'bg-emerald-50 text-emerald-700 border-emerald-200';
    default:
      return 'bg-gray-100 text-gray-700 border-gray-200';
  }
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function SummaryCards({ jobs }: { jobs: CronJob[] }) {
  const list = Array.isArray(jobs) ? jobs : [];
  const total = list.length;
  const active = list.filter((j) => j?.status === 'active').length;
  const inactive = list.filter((j) => j?.status === 'inactive').length;
  const recentlyRun = list.filter((j) => Boolean(j?.last_run_at)).length;

  const cards = [
    {
      label: 'Total Jobs',
      value: total,
      icon: <CalendarClock className="h-5 w-5 text-brand-600" />,
      iconBg: 'bg-brand-50 border border-brand-200',
    },
    {
      label: 'Active',
      value: active,
      icon: <CheckCircle2 className="h-5 w-5 text-emerald-600" />,
      iconBg: 'bg-emerald-50 border border-emerald-200',
    },
    {
      label: 'Inactive',
      value: inactive,
      icon: <XCircle className="h-5 w-5 text-gray-600" />,
      iconBg: 'bg-gray-100 border border-gray-200',
    },
    {
      label: 'Executed',
      value: recentlyRun,
      icon: <Activity className="h-5 w-5 text-amber-600" />,
      iconBg: 'bg-amber-50 border border-amber-200',
    },
  ];

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      {cards.map((c) => (
        <Card key={c.label} className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <CardContent className="p-5 flex items-center gap-4">
            <div className={`flex-shrink-0 h-10 w-10 rounded-md flex items-center justify-center ${c.iconBg}`}>
              {c.icon}
            </div>
            <div>
              <p className="text-sm text-gray-500">{c.label}</p>
              <p className="text-2xl font-semibold text-gray-900">{c.value}</p>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

function CronJobFormModal({
  open,
  onClose,
  onSubmit,
  initialData,
  loading,
}: {
  open: boolean;
  onClose: () => void;
  onSubmit: (data: CronJobFormData) => void;
  initialData?: CronJobFormData;
  loading: boolean;
}) {
  const [form, setForm] = useState<CronJobFormData>(initialData ?? DEFAULT_FORM);

  useEffect(() => {
    if (open) setForm(initialData ?? DEFAULT_FORM);
  }, [open, initialData]);

  if (!open) return null;

  const handleChange = (field: keyof CronJobFormData, value: string) =>
    setForm((prev) => ({ ...prev, [field]: value }));

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSubmit(form);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-gray-900/50"
        onClick={onClose}
      />
      {/* Modal */}
      <div
        role="dialog"
        aria-modal="true"
        aria-label={initialData ? 'Edit Cron Job' : 'Add Cron Job'}
        className="relative w-full max-w-lg mx-4 bg-white border border-gray-200 rounded-lg shadow-lg"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-200">
          <h2 className="text-sm font-semibold text-gray-900">
            {initialData ? 'Edit Cron Job' : 'Add Cron Job'}
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close dialog"
            className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Body */}
        <form onSubmit={handleSubmit} className="px-5 py-4 space-y-4">
          {/* Name */}
          <div>
            <label htmlFor="cron-name" className="block text-sm font-medium text-gray-700 mb-1">
              Name
            </label>
            <Input
              id="cron-name"
              value={form.name}
              onChange={(e) => handleChange('name', e.target.value)}
              placeholder="e.g. Database Backup"
              required
              className={INPUT_CLASS}
            />
          </div>

          {/* Command */}
          <div>
            <label htmlFor="cron-command" className="block text-sm font-medium text-gray-700 mb-1">
              Command
            </label>
            <textarea
              id="cron-command"
              value={form.command}
              onChange={(e) => handleChange('command', e.target.value)}
              placeholder="e.g. /usr/bin/php /var/www/artisan schedule:run"
              required
              rows={3}
              className="flex w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus-visible:border-brand-500 focus-visible:ring-1 focus-visible:ring-brand-500 focus-visible:outline-none font-mono resize-none"
            />
          </div>

          {/* Schedule */}
          <div>
            <label htmlFor="cron-schedule" className="block text-sm font-medium text-gray-700 mb-1">
              Schedule (Cron Expression)
            </label>
            <Input
              id="cron-schedule"
              value={form.schedule}
              onChange={(e) => handleChange('schedule', e.target.value)}
              placeholder="e.g. */5 * * * *"
              required
              className={`${INPUT_CLASS} font-mono`}
            />
            <p className="mt-1 text-xs text-gray-500">
              Standard cron format: minute hour day month weekday
            </p>
          </div>

          {/* Type */}
          <div>
            <label htmlFor="cron-type" className="block text-sm font-medium text-gray-700 mb-1">
              Type
            </label>
            <Select
              value={form.type}
              onValueChange={(v) => handleChange('type', v)}
            >
              <SelectTrigger id="cron-type" aria-label="Cron job type" className={SELECT_TRIGGER_CLASS}>
                <SelectValue placeholder="Select type" />
              </SelectTrigger>
              <SelectContent className={SELECT_CONTENT_CLASS}>
                {CRON_TYPES.map((t) => (
                  <SelectItem key={t} value={t} className={SELECT_ITEM_CLASS}>
                    <span className="flex items-center gap-2">
                      {typeIcon(t)}
                      {t.charAt(0).toUpperCase() + t.slice(1)}
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Server ID */}
          <div>
            <label htmlFor="cron-server-id" className="block text-sm font-medium text-gray-700 mb-1">
              Server ID
            </label>
            <Input
              id="cron-server-id"
              value={form.server_id}
              onChange={(e) => handleChange('server_id', e.target.value)}
              placeholder="UUID of target server"
              required
              className={`${INPUT_CLASS} font-mono text-xs`}
            />
          </div>

          {/* Footer */}
          <div className="flex items-center justify-end gap-3 pt-2">
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              className={BTN_SECONDARY}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={loading} className={BTN_PRIMARY}>
              {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {initialData ? 'Save Changes' : 'Create Job'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}

function DeleteConfirmModal({
  open,
  jobName,
  onConfirm,
  onCancel,
  loading,
}: {
  open: boolean;
  jobName: string;
  onConfirm: () => void;
  onCancel: () => void;
  loading: boolean;
}) {
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div
        className="absolute inset-0 bg-gray-900/50"
        onClick={onCancel}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Delete Cron Job"
        className="relative w-full max-w-md mx-4 bg-white border border-gray-200 rounded-lg shadow-lg"
      >
        <div className="px-5 py-5">
          <div className="flex items-center gap-3 mb-4">
            <div className="flex-shrink-0 w-10 h-10 rounded-md bg-red-50 border border-red-200 flex items-center justify-center">
              <AlertCircle className="h-5 w-5 text-red-600" />
            </div>
            <h3 className="text-sm font-semibold text-gray-900">
              Delete Cron Job
            </h3>
          </div>
          <p className="text-sm text-gray-600 mb-6">
            Are you sure you want to delete{' '}
            <strong className="text-gray-900">{jobName}</strong>? This action
            cannot be undone.
          </p>
          <div className="flex items-center justify-end gap-3">
            <Button variant="outline" onClick={onCancel} className={BTN_SECONDARY}>
              Cancel
            </Button>
            <Button onClick={onConfirm} disabled={loading} className={BTN_DANGER}>
              {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Delete
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function CronPage() {
  const [jobs, setJobs] = useState<CronJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [lastRefreshed, setLastRefreshed] = useState<string>('');

  // Filters
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [typeFilter, setTypeFilter] = useState<string>('all');

  // Modal state
  const [showForm, setShowForm] = useState(false);
  const [editingJob, setEditingJob] = useState<CronJob | null>(null);
  const [formLoading, setFormLoading] = useState(false);

  // Delete state
  const [deletingJob, setDeletingJob] = useState<CronJob | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  // Action loading states
  const [actionLoading, setActionLoading] = useState<Record<string, boolean>>(
    {}
  );

  // --------------------------------------------------
  // Data fetching
  // --------------------------------------------------

  const fetchJobs = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await cronApi.list();
      setJobs(Array.isArray(res?.data?.data) ? res.data.data : []);
      setLastRefreshed(new Date().toLocaleTimeString());
    } catch (err: any) {
      setJobs([]);
      setError(err?.response?.data?.message ?? 'Failed to load cron jobs');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchJobs();
  }, [fetchJobs]);

  // --------------------------------------------------
  // CRUD handlers
  // --------------------------------------------------

  const handleCreate = async (data: CronJobFormData) => {
    try {
      setFormLoading(true);
      setActionError(null);
      await cronApi.create(data);
      setShowForm(false);
      await fetchJobs();
    } catch (err: any) {
      setActionError(err?.response?.data?.message ?? 'Failed to create cron job');
    } finally {
      setFormLoading(false);
    }
  };

  const handleUpdate = async (data: CronJobFormData) => {
    if (!editingJob) return;
    try {
      setFormLoading(true);
      setActionError(null);
      await cronApi.update(editingJob.id, data);
      setEditingJob(null);
      setShowForm(false);
      await fetchJobs();
    } catch (err: any) {
      setActionError(err?.response?.data?.message ?? 'Failed to update cron job');
    } finally {
      setFormLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!deletingJob) return;
    try {
      setDeleteLoading(true);
      setActionError(null);
      await cronApi.delete(deletingJob.id);
      setDeletingJob(null);
      await fetchJobs();
    } catch (err: any) {
      setActionError(err?.response?.data?.message ?? 'Failed to delete cron job');
    } finally {
      setDeleteLoading(false);
    }
  };

  const handleToggle = async (job: CronJob) => {
    try {
      setActionLoading((prev) => ({ ...prev, [job.id]: true }));
      setActionError(null);
      await api.post(`/api/v1/cron/${job.id}/toggle`);
      await fetchJobs();
    } catch (err: any) {
      setActionError(err?.response?.data?.message ?? 'Failed to toggle cron job');
    } finally {
      setActionLoading((prev) => ({ ...prev, [job.id]: false }));
    }
  };

  const handleRunNow = async (job: CronJob) => {
    try {
      setActionLoading((prev) => ({ ...prev, [`run-${job.id}`]: true }));
      setActionError(null);
      await api.post(`/api/v1/cron/${job.id}/run`);
      await fetchJobs();
    } catch (err: any) {
      setActionError(err?.response?.data?.message ?? 'Failed to run cron job');
    } finally {
      setActionLoading((prev) => ({ ...prev, [`run-${job.id}`]: false }));
    }
  };

  // --------------------------------------------------
  // Filtering
  // --------------------------------------------------

  const safeJobs = Array.isArray(jobs) ? jobs : [];

  const filteredJobs = safeJobs.filter((job) => {
    if (!job) return false;
    const query = searchQuery.toLowerCase();
    const matchesSearch =
      searchQuery === '' ||
      (job.name ?? '').toLowerCase().includes(query) ||
      (job.command ?? '').toLowerCase().includes(query) ||
      (job.schedule ?? '').includes(searchQuery);

    const matchesStatus =
      statusFilter === 'all' || job.status === statusFilter;

    const matchesType = typeFilter === 'all' || job.type === typeFilter;

    return matchesSearch && matchesStatus && matchesType;
  });

  // --------------------------------------------------
  // Render
  // --------------------------------------------------

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900 flex items-center gap-2">
            <Clock className="h-5 w-5 text-gray-500" />
            Cron Jobs
          </h1>
          <p className="text-sm text-gray-600 mt-1">
            Manage scheduled tasks and automated scripts
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={fetchJobs}
            className={BTN_SECONDARY}
          >
            <RefreshCw className="h-4 w-4 mr-1" />
            Refresh
          </Button>
          <Button
            size="sm"
            onClick={() => {
              setEditingJob(null);
              setShowForm(true);
            }}
            className={BTN_PRIMARY}
          >
            <Plus className="h-4 w-4 mr-1" />
            Add Cron Job
          </Button>
        </div>
      </div>

      {/* Summary cards */}
      <SummaryCards jobs={safeJobs} />

      {/* Filters bar */}
      <Card className="bg-white border border-gray-200 rounded-lg shadow-sm">
        <CardContent className="p-4">
          <div className="flex flex-col sm:flex-row gap-3">
            {/* Search */}
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400" />
              <Input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search by name, command, or schedule..."
                aria-label="Search cron jobs"
                className={`pl-9 ${INPUT_CLASS}`}
              />
            </div>

            {/* Status filter */}
            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger
                aria-label="Filter by status"
                className={`w-full sm:w-40 ${SELECT_TRIGGER_CLASS}`}
              >
                <Filter className="h-4 w-4 mr-2 text-gray-400" />
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent className={SELECT_CONTENT_CLASS}>
                <SelectItem value="all" className={SELECT_ITEM_CLASS}>
                  All Statuses
                </SelectItem>
                {CRON_STATUSES.map((s) => (
                  <SelectItem key={s} value={s} className={SELECT_ITEM_CLASS}>
                    {s.charAt(0).toUpperCase() + s.slice(1)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {/* Type filter */}
            <Select value={typeFilter} onValueChange={setTypeFilter}>
              <SelectTrigger
                aria-label="Filter by type"
                className={`w-full sm:w-40 ${SELECT_TRIGGER_CLASS}`}
              >
                <Terminal className="h-4 w-4 mr-2 text-gray-400" />
                <SelectValue placeholder="Type" />
              </SelectTrigger>
              <SelectContent className={SELECT_CONTENT_CLASS}>
                <SelectItem value="all" className={SELECT_ITEM_CLASS}>
                  All Types
                </SelectItem>
                {CRON_TYPES.map((t) => (
                  <SelectItem key={t} value={t} className={SELECT_ITEM_CLASS}>
                    {t.charAt(0).toUpperCase() + t.slice(1)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* Error state */}
      {error && (
        <div className="flex items-center gap-3 p-4 bg-red-50 border border-red-200 rounded-lg">
          <AlertCircle className="h-5 w-5 text-red-600 flex-shrink-0" />
          <p className="text-sm text-red-700">{error}</p>
          <Button
            variant="outline"
            size="sm"
            onClick={fetchJobs}
            className="ml-auto bg-white border border-red-300 text-red-700 hover:bg-red-50 rounded-md text-sm font-medium"
          >
            Retry
          </Button>
        </div>
      )}

      {/* Action error state */}
      {actionError && (
        <div className="flex items-center gap-3 p-4 bg-red-50 border border-red-200 rounded-lg">
          <AlertCircle className="h-5 w-5 text-red-600 flex-shrink-0" />
          <p className="text-sm text-red-700">{actionError}</p>
          <button
            type="button"
            onClick={() => setActionError(null)}
            aria-label="Dismiss error"
            className="ml-auto rounded-md p-1 text-red-600 hover:bg-red-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      {/* Loading state */}
      {loading && (
        <div className="flex items-center justify-center py-20">
          <Loader2 className="h-8 w-8 animate-spin text-brand-600" />
        </div>
      )}

      {/* Empty state */}
      {!loading && !error && safeJobs.length === 0 && (
        <Card className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <CardContent className="py-16 text-center">
            <Clock className="mx-auto text-gray-400" size={40} />
            <h3 className="mt-4 text-sm font-semibold text-gray-900">
              No cron jobs
            </h3>
            <p className="mt-2 text-sm text-gray-600">
              Add your first cron job to automate scheduled tasks
            </p>
            <Button
              className={`mt-6 ${BTN_PRIMARY}`}
              onClick={() => {
                setEditingJob(null);
                setShowForm(true);
              }}
            >
              <Plus className="h-4 w-4 mr-1" />
              Add Cron Job
            </Button>
          </CardContent>
        </Card>
      )}

      {/* No results after filtering */}
      {!loading && !error && safeJobs.length > 0 && filteredJobs.length === 0 && (
        <Card className="bg-white border border-gray-200 rounded-lg shadow-sm">
          <CardContent className="py-12 text-center">
            <Search className="mx-auto text-gray-400" size={32} />
            <h3 className="mt-4 text-sm font-semibold text-gray-900">
              No matching jobs
            </h3>
            <p className="mt-2 text-sm text-gray-600">
              Try adjusting your search or filter criteria
            </p>
          </CardContent>
        </Card>
      )}

      {/* Jobs table */}
      {!loading && filteredJobs.length > 0 && (
        <Card className="bg-white border border-gray-200 rounded-lg shadow-sm overflow-hidden">
          <div className="px-5 py-4 border-b border-gray-200">
            <h2 className="text-sm font-semibold text-gray-900">Scheduled Jobs</h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">
                    Name
                  </th>
                  <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">
                    Command
                  </th>
                  <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">
                    Schedule
                  </th>
                  <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">
                    Type
                  </th>
                  <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">
                    Status
                  </th>
                  <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">
                    Last Run
                  </th>
                  <th className="text-left px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">
                    Next Run
                  </th>
                  <th className="text-right px-4 py-3 text-xs font-semibold uppercase tracking-wide text-gray-500">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody>
                {filteredJobs.map((job) => {
                  const isToggling = actionLoading[job.id] ?? false;
                  const isRunning = actionLoading[`run-${job.id}`] ?? false;

                  return (
                    <tr
                      key={job.id}
                      className="border-b border-gray-100 hover:bg-gray-50 transition-colors"
                    >
                      {/* Name */}
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <div className="flex-shrink-0 w-8 h-8 rounded-md bg-gray-100 border border-gray-200 text-gray-600 flex items-center justify-center">
                            {typeIcon(job.type)}
                          </div>
                          <div>
                            <p className="font-medium text-gray-900">
                              {job.name}
                            </p>
                            <p className="text-xs text-gray-500 font-mono truncate max-w-[180px]">
                              {(job.id ?? '').slice(0, 8)}…
                            </p>
                          </div>
                        </div>
                      </td>

                      {/* Command */}
                      <td className="px-4 py-3">
                        <code className="text-xs text-gray-700 bg-gray-50 border border-gray-200 px-2 py-1 rounded font-mono block max-w-[260px] truncate">
                          {job.command}
                        </code>
                      </td>

                      {/* Schedule */}
                      <td className="px-4 py-3">
                        <span className="font-mono text-xs text-brand-700 bg-brand-50 border border-brand-200 px-2 py-1 rounded">
                          {job.schedule}
                        </span>
                      </td>

                      {/* Type */}
                      <td className="px-4 py-3">
                        <span
                          className={`inline-flex items-center gap-1.5 text-xs font-medium px-2 py-0.5 rounded-md border ${typeBadgeColor(
                            job.type
                          )}`}
                        >
                          {typeIcon(job.type)}
                          {job.type}
                        </span>
                      </td>

                      {/* Status */}
                      <td className="px-4 py-3">
                        {job.status === 'active' ? (
                          <Badge className="rounded-md border border-emerald-200 bg-emerald-50 text-emerald-700 px-2 py-0.5 text-xs font-medium hover:bg-emerald-50">
                            <CheckCircle2 className="h-3 w-3 mr-1" />
                            Active
                          </Badge>
                        ) : (
                          <Badge className="rounded-md border border-gray-200 bg-gray-100 text-gray-600 px-2 py-0.5 text-xs font-medium hover:bg-gray-100">
                            <XCircle className="h-3 w-3 mr-1" />
                            Inactive
                          </Badge>
                        )}
                      </td>

                      {/* Last Run */}
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1.5 text-gray-700">
                          <Timer className="h-3.5 w-3.5 text-gray-400" />
                          <span className="text-xs" suppressHydrationWarning>
                            {relativeTime(job.last_run_at ?? null)}
                          </span>
                        </div>
                      </td>

                      {/* Next Run */}
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1.5 text-gray-700">
                          <Clock className="h-3.5 w-3.5 text-gray-400" />
                          <span className="text-xs" suppressHydrationWarning>
                            {formatDate(job.next_run_at ?? null)}
                          </span>
                        </div>
                      </td>

                      {/* Actions */}
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1">
                          {/* Run Now */}
                          <Button
                            variant="ghost"
                            size="icon"
                            title="Run Now"
                            aria-label={`Run ${job.name} now`}
                            disabled={isRunning || job.status !== 'active'}
                            onClick={() => handleRunNow(job)}
                            className="h-8 w-8 text-gray-500 hover:text-emerald-700 hover:bg-emerald-50 disabled:opacity-40"
                          >
                            {isRunning ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <Play className="h-4 w-4" />
                            )}
                          </Button>

                          {/* Toggle */}
                          <Button
                            variant="ghost"
                            size="icon"
                            title={
                              job.status === 'active' ? 'Disable' : 'Enable'
                            }
                            aria-label={
                              job.status === 'active'
                                ? `Disable ${job.name}`
                                : `Enable ${job.name}`
                            }
                            disabled={isToggling}
                            onClick={() => handleToggle(job)}
                            className="h-8 w-8 text-gray-500 hover:text-amber-700 hover:bg-amber-50"
                          >
                            {isToggling ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : job.status === 'active' ? (
                              <Pause className="h-4 w-4" />
                            ) : (
                              <Play className="h-4 w-4" />
                            )}
                          </Button>

                          {/* Edit */}
                          <Button
                            variant="ghost"
                            size="icon"
                            title="Edit"
                            aria-label={`Edit ${job.name}`}
                            onClick={() => {
                              setEditingJob(job);
                              setShowForm(true);
                            }}
                            className="h-8 w-8 text-gray-500 hover:text-brand-700 hover:bg-brand-50"
                          >
                            <Edit className="h-4 w-4" />
                          </Button>

                          {/* Delete */}
                          <Button
                            variant="ghost"
                            size="icon"
                            title="Delete"
                            aria-label={`Delete ${job.name}`}
                            onClick={() => setDeletingJob(job)}
                            className="h-8 w-8 text-gray-500 hover:text-red-700 hover:bg-red-50"
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* Table footer */}
          <div className="px-4 py-3 border-t border-gray-200 bg-gray-50 flex items-center justify-between text-xs text-gray-500">
            <span>
              Showing {filteredJobs.length} of {safeJobs.length} cron jobs
            </span>
            <span suppressHydrationWarning>
              {lastRefreshed ? `Last refreshed: ${lastRefreshed}` : ''}
            </span>
          </div>
        </Card>
      )}

      {/* Create / Edit modal */}
      <CronJobFormModal
        open={showForm}
        onClose={() => {
          setShowForm(false);
          setEditingJob(null);
        }}
        onSubmit={editingJob ? handleUpdate : handleCreate}
        initialData={
          editingJob
            ? {
                name: editingJob.name,
                command: editingJob.command,
                schedule: editingJob.schedule,
                type: editingJob.type,
                server_id: editingJob.server_id,
              }
            : undefined
        }
        loading={formLoading}
      />

      {/* Delete confirmation modal */}
      <DeleteConfirmModal
        open={!!deletingJob}
        jobName={deletingJob?.name ?? ''}
        onConfirm={handleDelete}
        onCancel={() => setDeletingJob(null)}
        loading={deleteLoading}
      />
    </div>
  );
}
