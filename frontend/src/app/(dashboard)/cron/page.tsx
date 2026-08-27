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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatDate(iso: string | null): string {
  if (!iso) return '—';
  const d = new Date(iso);
  return d.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function relativeTime(iso: string | null): string {
  if (!iso) return 'Never';
  const diff = Date.now() - new Date(iso).getTime();
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
      return 'bg-dark-700 text-dark-200 border-dark-600';
    case 'php':
      return 'bg-purple-900/40 text-purple-300 border-purple-700';
    case 'url':
      return 'bg-blue-900/40 text-blue-300 border-blue-700';
    case 'nodejs':
      return 'bg-green-900/40 text-green-300 border-green-700';
    default:
      return 'bg-dark-700 text-dark-200 border-dark-600';
  }
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function SummaryCards({ jobs }: { jobs: CronJob[] }) {
  const total = jobs.length;
  const active = jobs.filter((j) => j.status === 'active').length;
  const inactive = jobs.filter((j) => j.status === 'inactive').length;
  const recentlyRun = jobs.filter((j) => j.last_run_at).length;

  const cards = [
    {
      label: 'Total Jobs',
      value: total,
      icon: <CalendarClock className="h-5 w-5 text-primary-400" />,
      bg: 'bg-primary-900/20 border-primary-800/40',
    },
    {
      label: 'Active',
      value: active,
      icon: <CheckCircle2 className="h-5 w-5 text-green-400" />,
      bg: 'bg-green-900/20 border-green-800/40',
    },
    {
      label: 'Inactive',
      value: inactive,
      icon: <XCircle className="h-5 w-5 text-red-400" />,
      bg: 'bg-red-900/20 border-red-800/40',
    },
    {
      label: 'Executed',
      value: recentlyRun,
      icon: <Activity className="h-5 w-5 text-yellow-400" />,
      bg: 'bg-yellow-900/20 border-yellow-800/40',
    },
  ];

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      {cards.map((c) => (
        <Card key={c.label} className={`${c.bg} border`}>
          <CardContent className="p-4 flex items-center gap-4">
            <div className="flex-shrink-0">{c.icon}</div>
            <div>
              <p className="text-sm text-dark-400">{c.label}</p>
              <p className="text-2xl font-bold text-dark-50">{c.value}</p>
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
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
      />
      {/* Modal */}
      <div className="relative w-full max-w-lg mx-4 bg-dark-900 border border-dark-700 rounded-xl shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-dark-700">
          <h2 className="text-lg font-semibold text-dark-50">
            {initialData ? 'Edit Cron Job' : 'Add Cron Job'}
          </h2>
          <button
            onClick={onClose}
            className="text-dark-400 hover:text-dark-200 transition-colors"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Body */}
        <form onSubmit={handleSubmit} className="px-6 py-4 space-y-4">
          {/* Name */}
          <div>
            <label className="block text-sm font-medium text-dark-300 mb-1">
              Name
            </label>
            <Input
              value={form.name}
              onChange={(e) => handleChange('name', e.target.value)}
              placeholder="e.g. Database Backup"
              required
              className="bg-dark-800 border-dark-600 text-dark-100 placeholder:text-dark-500"
            />
          </div>

          {/* Command */}
          <div>
            <label className="block text-sm font-medium text-dark-300 mb-1">
              Command
            </label>
            <textarea
              value={form.command}
              onChange={(e) => handleChange('command', e.target.value)}
              placeholder="e.g. /usr/bin/php /var/www/artisan schedule:run"
              required
              rows={3}
              className="flex w-full rounded-md border border-dark-600 bg-dark-800 px-3 py-2 text-sm text-dark-100 placeholder:text-dark-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 focus-visible:ring-offset-dark-900 font-mono resize-none"
            />
          </div>

          {/* Schedule */}
          <div>
            <label className="block text-sm font-medium text-dark-300 mb-1">
              Schedule (Cron Expression)
            </label>
            <Input
              value={form.schedule}
              onChange={(e) => handleChange('schedule', e.target.value)}
              placeholder="e.g. */5 * * * *"
              required
              className="bg-dark-800 border-dark-600 text-dark-100 placeholder:text-dark-500 font-mono"
            />
            <p className="mt-1 text-xs text-dark-500">
              Standard cron format: minute hour day month weekday
            </p>
          </div>

          {/* Type */}
          <div>
            <label className="block text-sm font-medium text-dark-300 mb-1">
              Type
            </label>
            <Select
              value={form.type}
              onValueChange={(v) => handleChange('type', v)}
            >
              <SelectTrigger className="bg-dark-800 border-dark-600 text-dark-100">
                <SelectValue placeholder="Select type" />
              </SelectTrigger>
              <SelectContent className="bg-dark-800 border-dark-600">
                {CRON_TYPES.map((t) => (
                  <SelectItem
                    key={t}
                    value={t}
                    className="text-dark-100 focus:bg-dark-700 focus:text-dark-50"
                  >
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
            <label className="block text-sm font-medium text-dark-300 mb-1">
              Server ID
            </label>
            <Input
              value={form.server_id}
              onChange={(e) => handleChange('server_id', e.target.value)}
              placeholder="UUID of target server"
              required
              className="bg-dark-800 border-dark-600 text-dark-100 placeholder:text-dark-500 font-mono text-xs"
            />
          </div>

          {/* Footer */}
          <div className="flex items-center justify-end gap-3 pt-2">
            <Button
              type="button"
              variant="ghost"
              onClick={onClose}
              className="text-dark-300 hover:text-dark-100 hover:bg-dark-700"
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={loading}
              className="bg-primary-600 hover:bg-primary-700 text-white"
            >
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
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onCancel}
      />
      <div className="relative w-full max-w-md mx-4 bg-dark-900 border border-dark-700 rounded-xl shadow-2xl">
        <div className="px-6 py-5">
          <div className="flex items-center gap-3 mb-4">
            <div className="flex-shrink-0 w-10 h-10 rounded-full bg-red-900/30 flex items-center justify-center">
              <AlertCircle className="h-5 w-5 text-red-400" />
            </div>
            <h3 className="text-lg font-semibold text-dark-50">
              Delete Cron Job
            </h3>
          </div>
          <p className="text-dark-300 text-sm mb-6">
            Are you sure you want to delete{' '}
            <strong className="text-dark-100">{jobName}</strong>? This action
            cannot be undone.
          </p>
          <div className="flex items-center justify-end gap-3">
            <Button
              variant="ghost"
              onClick={onCancel}
              className="text-dark-300 hover:text-dark-100 hover:bg-dark-700"
            >
              Cancel
            </Button>
            <Button
              onClick={onConfirm}
              disabled={loading}
              className="bg-red-600 hover:bg-red-700 text-white"
            >
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
      setJobs(res.data.data ?? []);
    } catch (err: any) {
      setError(err.response?.data?.message ?? 'Failed to load cron jobs');
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
      await cronApi.create(data);
      setShowForm(false);
      await fetchJobs();
    } catch (err: any) {
      alert(err.response?.data?.message ?? 'Failed to create cron job');
    } finally {
      setFormLoading(false);
    }
  };

  const handleUpdate = async (data: CronJobFormData) => {
    if (!editingJob) return;
    try {
      setFormLoading(true);
      await cronApi.update(editingJob.id, data);
      setEditingJob(null);
      setShowForm(false);
      await fetchJobs();
    } catch (err: any) {
      alert(err.response?.data?.message ?? 'Failed to update cron job');
    } finally {
      setFormLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!deletingJob) return;
    try {
      setDeleteLoading(true);
      await cronApi.delete(deletingJob.id);
      setDeletingJob(null);
      await fetchJobs();
    } catch (err: any) {
      alert(err.response?.data?.message ?? 'Failed to delete cron job');
    } finally {
      setDeleteLoading(false);
    }
  };

  const handleToggle = async (job: CronJob) => {
    try {
      setActionLoading((prev) => ({ ...prev, [job.id]: true }));
      await api.post(`/api/v1/cron/${job.id}/toggle`);
      await fetchJobs();
    } catch (err: any) {
      alert(err.response?.data?.message ?? 'Failed to toggle cron job');
    } finally {
      setActionLoading((prev) => ({ ...prev, [job.id]: false }));
    }
  };

  const handleRunNow = async (job: CronJob) => {
    try {
      setActionLoading((prev) => ({ ...prev, [`run-${job.id}`]: true }));
      await api.post(`/api/v1/cron/${job.id}/run`);
      await fetchJobs();
    } catch (err: any) {
      alert(err.response?.data?.message ?? 'Failed to run cron job');
    } finally {
      setActionLoading((prev) => ({ ...prev, [`run-${job.id}`]: false }));
    }
  };

  // --------------------------------------------------
  // Filtering
  // --------------------------------------------------

  const filteredJobs = jobs.filter((job) => {
    const matchesSearch =
      searchQuery === '' ||
      job.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      job.command.toLowerCase().includes(searchQuery.toLowerCase()) ||
      job.schedule.includes(searchQuery);

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
          <h1 className="text-2xl font-bold text-dark-50 flex items-center gap-2">
            <Clock className="h-6 w-6 text-primary-400" />
            Cron Jobs
          </h1>
          <p className="text-dark-400 mt-1">
            Manage scheduled tasks and automated scripts
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={fetchJobs}
            className="border-dark-600 text-dark-300 hover:bg-dark-700 hover:text-dark-100"
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
            className="bg-primary-600 hover:bg-primary-700 text-white"
          >
            <Plus className="h-4 w-4 mr-1" />
            Add Cron Job
          </Button>
        </div>
      </div>

      {/* Summary cards */}
      <SummaryCards jobs={jobs} />

      {/* Filters bar */}
      <Card className="bg-dark-900 border-dark-700">
        <CardContent className="p-4">
          <div className="flex flex-col sm:flex-row gap-3">
            {/* Search */}
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-dark-500" />
              <Input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search by name, command, or schedule..."
                className="pl-9 bg-dark-800 border-dark-600 text-dark-100 placeholder:text-dark-500"
              />
            </div>

            {/* Status filter */}
            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger className="w-full sm:w-40 bg-dark-800 border-dark-600 text-dark-100">
                <Filter className="h-4 w-4 mr-2 text-dark-500" />
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent className="bg-dark-800 border-dark-600">
                <SelectItem
                  value="all"
                  className="text-dark-100 focus:bg-dark-700 focus:text-dark-50"
                >
                  All Statuses
                </SelectItem>
                {CRON_STATUSES.map((s) => (
                  <SelectItem
                    key={s}
                    value={s}
                    className="text-dark-100 focus:bg-dark-700 focus:text-dark-50"
                  >
                    {s.charAt(0).toUpperCase() + s.slice(1)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {/* Type filter */}
            <Select value={typeFilter} onValueChange={setTypeFilter}>
              <SelectTrigger className="w-full sm:w-40 bg-dark-800 border-dark-600 text-dark-100">
                <Terminal className="h-4 w-4 mr-2 text-dark-500" />
                <SelectValue placeholder="Type" />
              </SelectTrigger>
              <SelectContent className="bg-dark-800 border-dark-600">
                <SelectItem
                  value="all"
                  className="text-dark-100 focus:bg-dark-700 focus:text-dark-50"
                >
                  All Types
                </SelectItem>
                {CRON_TYPES.map((t) => (
                  <SelectItem
                    key={t}
                    value={t}
                    className="text-dark-100 focus:bg-dark-700 focus:text-dark-50"
                  >
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
        <div className="flex items-center gap-3 p-4 bg-red-900/20 border border-red-800/40 rounded-lg">
          <AlertCircle className="h-5 w-5 text-red-400 flex-shrink-0" />
          <p className="text-sm text-red-300">{error}</p>
          <Button
            variant="ghost"
            size="sm"
            onClick={fetchJobs}
            className="ml-auto text-red-300 hover:text-red-200 hover:bg-red-900/30"
          >
            Retry
          </Button>
        </div>
      )}

      {/* Loading state */}
      {loading && (
        <div className="flex items-center justify-center py-20">
          <Loader2 className="h-8 w-8 animate-spin text-primary-400" />
        </div>
      )}

      {/* Empty state */}
      {!loading && !error && jobs.length === 0 && (
        <Card className="bg-dark-900 border-dark-700">
          <CardContent className="py-16 text-center">
            <Clock className="mx-auto text-dark-600" size={48} />
            <h3 className="mt-4 text-lg font-medium text-dark-300">
              No cron jobs
            </h3>
            <p className="mt-2 text-dark-500 text-sm">
              Add your first cron job to automate scheduled tasks
            </p>
            <Button
              className="mt-6 bg-primary-600 hover:bg-primary-700 text-white"
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
      {!loading && !error && jobs.length > 0 && filteredJobs.length === 0 && (
        <Card className="bg-dark-900 border-dark-700">
          <CardContent className="py-12 text-center">
            <Search className="mx-auto text-dark-600" size={40} />
            <h3 className="mt-4 text-lg font-medium text-dark-300">
              No matching jobs
            </h3>
            <p className="mt-2 text-dark-500 text-sm">
              Try adjusting your search or filter criteria
            </p>
          </CardContent>
        </Card>
      )}

      {/* Jobs table */}
      {!loading && filteredJobs.length > 0 && (
        <Card className="bg-dark-900 border-dark-700 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-dark-700">
                  <th className="text-left px-4 py-3 text-dark-400 font-medium">
                    Name
                  </th>
                  <th className="text-left px-4 py-3 text-dark-400 font-medium">
                    Command
                  </th>
                  <th className="text-left px-4 py-3 text-dark-400 font-medium">
                    Schedule
                  </th>
                  <th className="text-left px-4 py-3 text-dark-400 font-medium">
                    Type
                  </th>
                  <th className="text-left px-4 py-3 text-dark-400 font-medium">
                    Status
                  </th>
                  <th className="text-left px-4 py-3 text-dark-400 font-medium">
                    Last Run
                  </th>
                  <th className="text-left px-4 py-3 text-dark-400 font-medium">
                    Next Run
                  </th>
                  <th className="text-right px-4 py-3 text-dark-400 font-medium">
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
                      className="border-b border-dark-800 hover:bg-dark-800/50 transition-colors"
                    >
                      {/* Name */}
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <div className="flex-shrink-0 w-8 h-8 rounded-lg bg-dark-800 flex items-center justify-center">
                            {typeIcon(job.type)}
                          </div>
                          <div>
                            <p className="font-medium text-dark-100">
                              {job.name}
                            </p>
                            <p className="text-xs text-dark-500 font-mono truncate max-w-[180px]">
                              {job.id.slice(0, 8)}…
                            </p>
                          </div>
                        </div>
                      </td>

                      {/* Command */}
                      <td className="px-4 py-3">
                        <code className="text-xs text-dark-300 bg-dark-800 px-2 py-1 rounded font-mono block max-w-[260px] truncate">
                          {job.command}
                        </code>
                      </td>

                      {/* Schedule */}
                      <td className="px-4 py-3">
                        <span className="font-mono text-xs text-primary-300 bg-primary-900/20 px-2 py-1 rounded">
                          {job.schedule}
                        </span>
                      </td>

                      {/* Type */}
                      <td className="px-4 py-3">
                        <span
                          className={`inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full border ${typeBadgeColor(
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
                          <Badge className="bg-green-900/30 text-green-400 border-green-700/50">
                            <CheckCircle2 className="h-3 w-3 mr-1" />
                            Active
                          </Badge>
                        ) : (
                          <Badge className="bg-dark-700 text-dark-400 border-dark-600">
                            <XCircle className="h-3 w-3 mr-1" />
                            Inactive
                          </Badge>
                        )}
                      </td>

                      {/* Last Run */}
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1.5 text-dark-300">
                          <Timer className="h-3.5 w-3.5 text-dark-500" />
                          <span className="text-xs">
                            {relativeTime(job.last_run_at)}
                          </span>
                        </div>
                      </td>

                      {/* Next Run */}
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1.5 text-dark-300">
                          <Clock className="h-3.5 w-3.5 text-dark-500" />
                          <span className="text-xs">
                            {formatDate(job.next_run_at)}
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
                            disabled={isRunning || job.status !== 'active'}
                            onClick={() => handleRunNow(job)}
                            className="h-8 w-8 text-dark-400 hover:text-green-400 hover:bg-green-900/20 disabled:opacity-30"
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
                            disabled={isToggling}
                            onClick={() => handleToggle(job)}
                            className="h-8 w-8 text-dark-400 hover:text-yellow-400 hover:bg-yellow-900/20"
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
                            onClick={() => {
                              setEditingJob(job);
                              setShowForm(true);
                            }}
                            className="h-8 w-8 text-dark-400 hover:text-blue-400 hover:bg-blue-900/20"
                          >
                            <Edit className="h-4 w-4" />
                          </Button>

                          {/* Delete */}
                          <Button
                            variant="ghost"
                            size="icon"
                            title="Delete"
                            onClick={() => setDeletingJob(job)}
                            className="h-8 w-8 text-dark-400 hover:text-red-400 hover:bg-red-900/20"
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
          <div className="px-4 py-3 border-t border-dark-700 flex items-center justify-between text-xs text-dark-500">
            <span>
              Showing {filteredJobs.length} of {jobs.length} cron jobs
            </span>
            <span>Last refreshed: {new Date().toLocaleTimeString()}</span>
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
