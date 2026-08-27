'use client';

import { useState, useEffect } from 'react';
import {
  HardDrive, RotateCcw, UploadCloud, Lock, Trash2, Activity,
  BarChart3, FileText, Bell, Settings
} from 'lucide-react';
import { jobApi } from '@/services/api';

interface Job {
  id: string;
  task_id: string;
  task_type: string;
  status: string;
  queue: string;
  payload: any;
  result: any;
  error: string;
  retry_count: number;
  max_retries: number;
  scheduled_at: string;
  started_at: string;
  completed_at: string;
  failed_at: string;
  created_at: string;
  updated_at: string;
}

interface JobStats {
  total: number;
  completed: number;
  failed: number;
  pending: number;
  active: number;
  by_type: Record<string, number>;
  by_queue: Record<string, number>;
  avg_runtime_seconds: number;
}

const CARD = 'bg-white border border-gray-200 rounded-lg shadow-sm';
const CARD_HEADER = 'px-5 py-4 border-b border-gray-200';
const TH = 'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
const TD = 'px-4 py-3 text-sm text-gray-700';
const ROW = 'border-b border-gray-100 hover:bg-gray-50';
const BADGE = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium';
const SELECT =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus:outline-none';
const LABEL = 'mb-1.5 block text-sm font-medium text-gray-700';
const LINK_BTN =
  'rounded-md px-2 py-1 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500';

export default function JobsPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [stats, setStats] = useState<JobStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [filter, setFilter] = useState({
    task_type: '',
    status: '',
    queue: '',
  });

  useEffect(() => {
    loadData();
  }, [filter]);

  const loadData = async () => {
    try {
      setLoading(true);
      setError('');
      const [jobsRes, statsRes] = await Promise.all([
        jobApi.list(filter),
        jobApi.getStats(),
      ]);
      setJobs(Array.isArray(jobsRes?.data?.data) ? jobsRes.data.data : []);
      setStats(statsRes?.data ?? null);
    } catch (error) {
      console.error('Failed to load jobs:', error);
      setError('Unable to load the job queue. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  const handleCancel = async (id: string) => {
    if (!confirm('Are you sure you want to cancel this job?')) return;
    try {
      await jobApi.cancel(id);
      loadData();
    } catch (error) {
      console.error('Failed to cancel job:', error);
      setError('Unable to cancel the job. Please try again.');
    }
  };

  const handleRetry = async (id: string) => {
    try {
      await jobApi.retry(id);
      loadData();
    } catch (error) {
      console.error('Failed to retry job:', error);
      setError('Unable to retry the job. Please try again.');
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this job?')) return;
    try {
      await jobApi.delete(id);
      loadData();
    } catch (error) {
      console.error('Failed to delete job:', error);
      setError('Unable to delete the job. Please try again.');
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'completed': return 'bg-emerald-50 text-emerald-700';
      case 'failed': return 'bg-red-50 text-red-700';
      case 'active': return 'bg-brand-50 text-brand-700';
      case 'pending': return 'bg-amber-50 text-amber-700';
      case 'cancelled': return 'bg-gray-100 text-gray-700';
      default: return 'bg-gray-100 text-gray-700';
    }
  };

  const getTaskTypeIcon = (type: string) => {
    switch (type) {
      case 'backup': return <HardDrive size={14} className="text-gray-500" />;
      case 'restore': return <RotateCcw size={14} className="text-gray-500" />;
      case 'deploy': return <UploadCloud size={14} className="text-gray-500" />;
      case 'ssl': return <Lock size={14} className="text-gray-500" />;
      case 'cleanup': return <Trash2 size={14} className="text-gray-500" />;
      case 'health_check': return <Activity size={14} className="text-gray-500" />;
      case 'metric_collect': return <BarChart3 size={14} className="text-gray-500" />;
      case 'log_rotate': return <FileText size={14} className="text-gray-500" />;
      case 'notification': return <Bell size={14} className="text-gray-500" />;
      default: return <Settings size={14} className="text-gray-500" />;
    }
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return '-';
    const d = new Date(dateStr);
    if (Number.isNaN(d.getTime())) return '-';
    return d.toLocaleString();
  };

  const formatDuration = (seconds: number) => {
    if (!seconds || !Number.isFinite(seconds)) return '-';
    if (seconds < 60) return `${seconds.toFixed(1)}s`;
    if (seconds < 3600) return `${(seconds / 60).toFixed(1)}m`;
    return `${(seconds / 3600).toFixed(1)}h`;
  };

  const jobDuration = (job: Job) => {
    if (!job.started_at || !job.completed_at) return '-';
    const start = new Date(job.started_at).getTime();
    const end = new Date(job.completed_at).getTime();
    if (Number.isNaN(start) || Number.isNaN(end)) return '-';
    return formatDuration((end - start) / 1000);
  };

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Job Queue</h1>
          <p className="mt-1 text-sm text-gray-600">Monitor background jobs, retries and queue throughput</p>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {/* Stats Cards */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-5 gap-4">
          <div className={`${CARD} p-4`}>
            <div className="text-xs font-medium uppercase tracking-wide text-gray-500">Total Jobs</div>
            <div className="mt-1 text-2xl font-semibold text-gray-900">{stats.total ?? 0}</div>
          </div>
          <div className={`${CARD} p-4`}>
            <div className="text-xs font-medium uppercase tracking-wide text-gray-500">Completed</div>
            <div className="mt-1 text-2xl font-semibold text-emerald-600">{stats.completed ?? 0}</div>
          </div>
          <div className={`${CARD} p-4`}>
            <div className="text-xs font-medium uppercase tracking-wide text-gray-500">Failed</div>
            <div className="mt-1 text-2xl font-semibold text-red-600">{stats.failed ?? 0}</div>
          </div>
          <div className={`${CARD} p-4`}>
            <div className="text-xs font-medium uppercase tracking-wide text-gray-500">Pending</div>
            <div className="mt-1 text-2xl font-semibold text-amber-600">{stats.pending ?? 0}</div>
          </div>
          <div className={`${CARD} p-4`}>
            <div className="text-xs font-medium uppercase tracking-wide text-gray-500">Avg Runtime</div>
            <div className="mt-1 text-2xl font-semibold text-gray-900">{formatDuration(stats.avg_runtime_seconds)}</div>
          </div>
        </div>
      )}

      {/* Filters */}
      <div className={CARD}>
        <div className={CARD_HEADER}>
          <h2 className="text-sm font-semibold text-gray-900">Filters</h2>
        </div>
        <div className="p-5">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label htmlFor="filter-task-type" className={LABEL}>Task Type</label>
              <select
                id="filter-task-type"
                value={filter.task_type}
                onChange={(e) => setFilter({ ...filter, task_type: e.target.value })}
                className={SELECT}
              >
                <option value="">All Types</option>
                <option value="backup">Backup</option>
                <option value="restore">Restore</option>
                <option value="deploy">Deploy</option>
                <option value="ssl">SSL</option>
                <option value="cleanup">Cleanup</option>
                <option value="health_check">Health Check</option>
                <option value="metric_collect">Metric Collect</option>
                <option value="log_rotate">Log Rotate</option>
                <option value="notification">Notification</option>
              </select>
            </div>
            <div>
              <label htmlFor="filter-status" className={LABEL}>Status</label>
              <select
                id="filter-status"
                value={filter.status}
                onChange={(e) => setFilter({ ...filter, status: e.target.value })}
                className={SELECT}
              >
                <option value="">All Statuses</option>
                <option value="pending">Pending</option>
                <option value="active">Active</option>
                <option value="completed">Completed</option>
                <option value="failed">Failed</option>
                <option value="cancelled">Cancelled</option>
              </select>
            </div>
            <div>
              <label htmlFor="filter-queue" className={LABEL}>Queue</label>
              <select
                id="filter-queue"
                value={filter.queue}
                onChange={(e) => setFilter({ ...filter, queue: e.target.value })}
                className={SELECT}
              >
                <option value="">All Queues</option>
                <option value="critical">Critical</option>
                <option value="default">Default</option>
                <option value="low">Low</option>
              </select>
            </div>
          </div>
        </div>
      </div>

      {/* Jobs Table */}
      <div className={`${CARD} overflow-hidden`}>
        <div className={CARD_HEADER}>
          <h2 className="text-sm font-semibold text-gray-900">Jobs</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full">
            <thead className="bg-gray-50 border-b border-gray-200">
              <tr>
                <th className={TH}>Type</th>
                <th className={TH}>Status</th>
                <th className={TH}>Queue</th>
                <th className={TH}>Retries</th>
                <th className={TH}>Created</th>
                <th className={TH}>Duration</th>
                <th className={`${TH} text-right`}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr>
                  <td colSpan={7} className="px-4 py-10 text-center text-sm text-gray-500">
                    Loading…
                  </td>
                </tr>
              ) : jobs.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-10 text-center text-sm text-gray-500">
                    No jobs found
                  </td>
                </tr>
              ) : (
                jobs.map((job) => (
                  <tr key={job.id} className={ROW}>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        {getTaskTypeIcon(job.task_type)}
                        <span className="text-sm font-medium text-gray-900">{job.task_type || '—'}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`${BADGE} ${getStatusColor(job.status)}`}>
                        {job.status || 'unknown'}
                      </span>
                      {job.error && (
                        <div className="mt-1 max-w-xs truncate text-xs text-red-700">{job.error}</div>
                      )}
                    </td>
                    <td className={TD}>{job.queue || '—'}</td>
                    <td className={TD}>
                      {job.retry_count ?? 0}/{job.max_retries ?? 0}
                    </td>
                    <td className={TD} suppressHydrationWarning>{formatDate(job.created_at)}</td>
                    <td className={TD} suppressHydrationWarning>{jobDuration(job)}</td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex justify-end gap-2">
                        {job.status === 'active' && (
                          <button
                            type="button"
                            onClick={() => handleCancel(job.id)}
                            className={`${LINK_BTN} text-amber-700 hover:bg-amber-50`}
                          >
                            Cancel
                          </button>
                        )}
                        {job.status === 'failed' && (
                          <button
                            type="button"
                            onClick={() => handleRetry(job.id)}
                            className={`${LINK_BTN} text-brand-700 hover:bg-brand-50`}
                          >
                            Retry
                          </button>
                        )}
                        {job.status !== 'active' && (
                          <button
                            type="button"
                            onClick={() => handleDelete(job.id)}
                            className={`${LINK_BTN} text-red-700 hover:bg-red-50`}
                          >
                            Delete
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Queue Stats */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div className={CARD}>
            <div className={CARD_HEADER}>
              <h2 className="text-sm font-semibold text-gray-900">Jobs by Type</h2>
            </div>
            <div className="p-5">
              {Object.keys(stats.by_type ?? {}).length === 0 ? (
                <p className="py-4 text-center text-sm text-gray-500">No data available</p>
              ) : (
                <div className="space-y-3">
                  {Object.entries(stats.by_type ?? {}).map(([type, count]) => (
                    <div key={type} className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        {getTaskTypeIcon(type)}
                        <span className="text-sm text-gray-700">{type}</span>
                      </div>
                      <span className="text-sm font-medium text-gray-900">{count}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
          <div className={CARD}>
            <div className={CARD_HEADER}>
              <h2 className="text-sm font-semibold text-gray-900">Jobs by Queue</h2>
            </div>
            <div className="p-5">
              {Object.keys(stats.by_queue ?? {}).length === 0 ? (
                <p className="py-4 text-center text-sm text-gray-500">No data available</p>
              ) : (
                <div className="space-y-3">
                  {Object.entries(stats.by_queue ?? {}).map(([queue, count]) => (
                    <div key={queue} className="flex items-center justify-between">
                      <span className="text-sm text-gray-700">{queue}</span>
                      <span className="text-sm font-medium text-gray-900">{count}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
