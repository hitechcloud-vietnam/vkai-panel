'use client';

import { useState, useEffect } from 'react';
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

export default function JobsPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [stats, setStats] = useState<JobStats | null>(null);
  const [loading, setLoading] = useState(true);
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
      const [jobsRes, statsRes] = await Promise.all([
        jobApi.list(filter),
        jobApi.getStats(),
      ]);
      setJobs(jobsRes.data.data || []);
      setStats(statsRes.data);
    } catch (error) {
      console.error('Failed to load jobs:', error);
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
    }
  };

  const handleRetry = async (id: string) => {
    try {
      await jobApi.retry(id);
      loadData();
    } catch (error) {
      console.error('Failed to retry job:', error);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this job?')) return;
    try {
      await jobApi.delete(id);
      loadData();
    } catch (error) {
      console.error('Failed to delete job:', error);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'completed': return 'bg-green-100 text-green-800';
      case 'failed': return 'bg-red-100 text-red-800';
      case 'active': return 'bg-blue-100 text-blue-800';
      case 'pending': return 'bg-yellow-100 text-yellow-800';
      case 'cancelled': return 'bg-gray-100 text-gray-800';
      default: return 'bg-gray-100 text-gray-800';
    }
  };

  const getTaskTypeIcon = (type: string) => {
    switch (type) {
      case 'backup': return '💾';
      case 'restore': return '♻️';
      case 'deploy': return '🚀';
      case 'ssl': return '🔒';
      case 'cleanup': return '🧹';
      case 'health_check': return '❤️';
      case 'metric_collect': return '📊';
      case 'log_rotate': return '📝';
      case 'notification': return '🔔';
      default: return '⚙️';
    }
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return '-';
    return new Date(dateStr).toLocaleString();
  };

  const formatDuration = (seconds: number) => {
    if (!seconds) return '-';
    if (seconds < 60) return `${seconds.toFixed(1)}s`;
    if (seconds < 3600) return `${(seconds / 60).toFixed(1)}m`;
    return `${(seconds / 3600).toFixed(1)}h`;
  };

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">Job Queue</h1>
      </div>

      {/* Stats Cards */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-5 gap-4">
          <div className="bg-white rounded-lg shadow p-4">
            <div className="text-sm text-gray-500">Total Jobs</div>
            <div className="text-2xl font-bold">{stats.total}</div>
          </div>
          <div className="bg-white rounded-lg shadow p-4">
            <div className="text-sm text-gray-500">Completed</div>
            <div className="text-2xl font-bold text-green-600">{stats.completed}</div>
          </div>
          <div className="bg-white rounded-lg shadow p-4">
            <div className="text-sm text-gray-500">Failed</div>
            <div className="text-2xl font-bold text-red-600">{stats.failed}</div>
          </div>
          <div className="bg-white rounded-lg shadow p-4">
            <div className="text-sm text-gray-500">Pending</div>
            <div className="text-2xl font-bold text-yellow-600">{stats.pending}</div>
          </div>
          <div className="bg-white rounded-lg shadow p-4">
            <div className="text-sm text-gray-500">Avg Runtime</div>
            <div className="text-2xl font-bold">{formatDuration(stats.avg_runtime_seconds)}</div>
          </div>
        </div>
      )}

      {/* Filters */}
      <div className="bg-white rounded-lg shadow p-4">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Task Type</label>
            <select
              value={filter.task_type}
              onChange={(e) => setFilter({ ...filter, task_type: e.target.value })}
              className="w-full border rounded-lg px-3 py-2"
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
            <label className="block text-sm font-medium text-gray-700 mb-1">Status</label>
            <select
              value={filter.status}
              onChange={(e) => setFilter({ ...filter, status: e.target.value })}
              className="w-full border rounded-lg px-3 py-2"
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
            <label className="block text-sm font-medium text-gray-700 mb-1">Queue</label>
            <select
              value={filter.queue}
              onChange={(e) => setFilter({ ...filter, queue: e.target.value })}
              className="w-full border rounded-lg px-3 py-2"
            >
              <option value="">All Queues</option>
              <option value="critical">Critical</option>
              <option value="default">Default</option>
              <option value="low">Low</option>
            </select>
          </div>
        </div>
      </div>

      {/* Jobs Table */}
      <div className="bg-white rounded-lg shadow overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Type</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Queue</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Retries</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Created</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Duration</th>
              <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200">
            {loading ? (
              <tr>
                <td colSpan={7} className="px-6 py-4 text-center text-gray-500">
                  Loading...
                </td>
              </tr>
            ) : jobs.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-6 py-4 text-center text-gray-500">
                  No jobs found
                </td>
              </tr>
            ) : (
              jobs.map((job) => (
                <tr key={job.id} className="hover:bg-gray-50">
                  <td className="px-6 py-4">
                    <div className="flex items-center">
                      <span className="mr-2">{getTaskTypeIcon(job.task_type)}</span>
                      <span className="font-medium">{job.task_type}</span>
                    </div>
                  </td>
                  <td className="px-6 py-4">
                    <span className={`px-2 py-1 rounded-full text-xs font-medium ${getStatusColor(job.status)}`}>
                      {job.status}
                    </span>
                    {job.error && (
                      <div className="text-xs text-red-500 mt-1 truncate max-w-xs">{job.error}</div>
                    )}
                  </td>
                  <td className="px-6 py-4 text-sm text-gray-500">{job.queue}</td>
                  <td className="px-6 py-4 text-sm text-gray-500">
                    {job.retry_count}/{job.max_retries}
                  </td>
                  <td className="px-6 py-4 text-sm text-gray-500">{formatDate(job.created_at)}</td>
                  <td className="px-6 py-4 text-sm text-gray-500">
                    {job.started_at && job.completed_at
                      ? formatDuration(
                          (new Date(job.completed_at).getTime() - new Date(job.started_at).getTime()) / 1000
                        )
                      : '-'}
                  </td>
                  <td className="px-6 py-4 text-right">
                    <div className="flex justify-end space-x-2">
                      {job.status === 'active' && (
                        <button
                          onClick={() => handleCancel(job.id)}
                          className="text-yellow-600 hover:text-yellow-900 text-sm"
                        >
                          Cancel
                        </button>
                      )}
                      {job.status === 'failed' && (
                        <button
                          onClick={() => handleRetry(job.id)}
                          className="text-blue-600 hover:text-blue-900 text-sm"
                        >
                          Retry
                        </button>
                      )}
                      {job.status !== 'active' && (
                        <button
                          onClick={() => handleDelete(job.id)}
                          className="text-red-600 hover:text-red-900 text-sm"
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

      {/* Queue Stats */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div className="bg-white rounded-lg shadow p-6">
            <h3 className="text-lg font-semibold mb-4">Jobs by Type</h3>
            <div className="space-y-3">
              {Object.entries(stats.by_type).map(([type, count]) => (
                <div key={type} className="flex items-center justify-between">
                  <div className="flex items-center">
                    <span className="mr-2">{getTaskTypeIcon(type)}</span>
                    <span>{type}</span>
                  </div>
                  <span className="font-medium">{count}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="bg-white rounded-lg shadow p-6">
            <h3 className="text-lg font-semibold mb-4">Jobs by Queue</h3>
            <div className="space-y-3">
              {Object.entries(stats.by_queue).map(([queue, count]) => (
                <div key={queue} className="flex items-center justify-between">
                  <span>{queue}</span>
                  <span className="font-medium">{count}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
