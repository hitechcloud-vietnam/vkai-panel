'use client';

import { useState, useEffect } from 'react';
import { Clock, Plus, Play, Pause, Trash2, RefreshCw, CheckCircle, XCircle, Timer, Zap, FileText, Settings, History, X } from 'lucide-react';

interface ScheduledTask {
  id: string;
  name: string;
  description: string;
  task_type: string;
  command: string;
  schedule: string;
  schedule_desc: string;
  is_enabled: boolean;
  priority: number;
  timeout: number;
  max_retries: number;
  tags: string[];
  last_run_at: string | null;
  last_status: string;
  run_count: number;
  fail_count: number;
  created_at: string;
}

interface TaskExecution {
  id: string;
  task_id: string;
  status: string;
  started_at: string | null;
  finished_at: string | null;
  duration: number;
  exit_code: number | null;
  output: string;
  error_output: string;
  triggered_by: string;
  created_at: string;
}

interface TaskStats {
  total_tasks: number;
  enabled_tasks: number;
  disabled_tasks: number;
  total_executions: number;
  success_rate: number;
  failed_today: number;
  run_today: number;
  avg_duration: number;
}

const CARD = 'bg-white border border-gray-200 rounded-lg shadow-sm';
const CARD_HEADER = 'px-5 py-4 border-b border-gray-200';
const INPUT =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus:outline-none';
const LABEL = 'mb-1.5 block text-sm font-medium text-gray-700';
const BTN_PRIMARY =
  'inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1 disabled:opacity-50';
const BTN_SECONDARY =
  'inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1';
const BADGE = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium';
const ICON_BTN =
  'rounded-md p-1.5 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500';
const ICON_DANGER =
  'rounded-md p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500';

function formatDateTime(value?: string | null): string {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString();
}

export default function ScheduledTasksPage() {
  const [stats, setStats] = useState<TaskStats | null>(null);
  const [tasks, setTasks] = useState<ScheduledTask[]>([]);
  const [executions, setExecutions] = useState<TaskExecution[]>([]);
  const [activeTab, setActiveTab] = useState<'tasks' | 'history'>('tasks');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [selectedTask, setSelectedTask] = useState<ScheduledTask | null>(null);
  const [taskExecutions, setTaskExecutions] = useState<TaskExecution[]>([]);
  const [newTask, setNewTask] = useState({
    name: '',
    description: '',
    task_type: 'command',
    command: '',
    schedule: '0 * * * *',
    priority: 2,
    timeout: 0,
    max_retries: 0,
    tags: '',
  });

  const getToken = () => {
    if (typeof window !== 'undefined') {
      try {
        const authStorage = localStorage.getItem('auth-storage');
        if (authStorage) {
          const parsed = JSON.parse(authStorage);
          return parsed?.state?.token || '';
        }
      } catch { return ''; }
    }
    return '';
  };

  const apiCall = async (url: string, options: RequestInit = {}) => {
    const token = getToken();
    const res = await fetch(`/api${url}`, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`,
        ...options.headers,
      },
    });
    if (!res.ok) throw new Error(`API error: ${res.status}`);
    return res.json();
  };

  const fetchAll = async () => {
    setLoading(true);
    setError('');
    try {
      const [statsRes, tasksRes, execsRes] = await Promise.all([
        apiCall('/v1/scheduled-tasks/stats').catch(() => ({ stats: null })),
        apiCall('/v1/scheduled-tasks').catch(() => ({ tasks: [] })),
        apiCall('/v1/scheduled-tasks/recent-executions?limit=50').catch(() => ({ executions: [] })),
      ]);
      setStats(statsRes?.stats ?? null);
      setTasks(Array.isArray(tasksRes?.tasks) ? tasksRes.tasks : []);
      setExecutions(Array.isArray(execsRes?.executions) ? execsRes.executions : []);
    } catch (err) {
      console.error('Failed to fetch data:', err);
      setError('Unable to load scheduled tasks. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchAll(); }, []);

  const handleCreateTask = async () => {
    try {
      await apiCall('/v1/scheduled-tasks', {
        method: 'POST',
        body: JSON.stringify({
          ...newTask,
          tags: newTask.tags.split(',').map(s => s.trim()).filter(Boolean),
        }),
      });
      setShowCreate(false);
      setNewTask({ name: '', description: '', task_type: 'command', command: '', schedule: '0 * * * *', priority: 2, timeout: 0, max_retries: 0, tags: '' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to create task:', err);
      setError('Unable to create the task. Please try again.');
    }
  };

  const handleToggleTask = async (id: string) => {
    try {
      await apiCall(`/v1/scheduled-tasks/${id}/toggle`, { method: 'POST' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to toggle task:', err);
      setError('Unable to change the task state. Please try again.');
    }
  };

  const handleRunTask = async (id: string) => {
    try {
      await apiCall(`/v1/scheduled-tasks/${id}/run`, { method: 'POST' });
      setTimeout(() => fetchAll(), 1000);
    } catch (err) {
      console.error('Failed to run task:', err);
      setError('Unable to run the task. Please try again.');
    }
  };

  const handleDeleteTask = async (id: string) => {
    if (!confirm('Delete this task and all its execution history?')) return;
    try {
      await apiCall(`/v1/scheduled-tasks/${id}`, { method: 'DELETE' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to delete task:', err);
      setError('Unable to delete the task. Please try again.');
    }
  };

  const fetchTaskExecutions = async (taskId: string) => {
    try {
      const res = await apiCall(`/v1/scheduled-tasks/${taskId}/executions?limit=20`);
      setTaskExecutions(Array.isArray(res?.executions) ? res.executions : []);
    } catch (err) {
      console.error('Failed to fetch executions:', err);
      setError('Unable to load the execution history. Please try again.');
    }
  };

  const getPriorityLabel = (p: number) => {
    switch (p) {
      case 1: return { label: 'Low', color: 'bg-gray-100 text-gray-700' };
      case 2: return { label: 'Normal', color: 'bg-brand-50 text-brand-700' };
      case 3: return { label: 'High', color: 'bg-amber-50 text-amber-700' };
      case 4: return { label: 'Critical', color: 'bg-red-50 text-red-700' };
      default: return { label: 'Normal', color: 'bg-brand-50 text-brand-700' };
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'success': return 'text-emerald-600';
      case 'failed': return 'text-red-600';
      case 'running': return 'text-brand-600';
      case 'timeout': return 'text-amber-600';
      default: return 'text-gray-600';
    }
  };

  const getTypeIcon = (type: string) => {
    switch (type) {
      case 'command': return <Zap size={16} className="text-gray-600" />;
      case 'script': return <FileText size={16} className="text-gray-600" />;
      case 'http': return <Settings size={16} className="text-gray-600" />;
      default: return <Clock size={16} className="text-gray-600" />;
    }
  };

  if (loading) {
    return (
      <div className={`${CARD} px-5 py-12 text-center text-sm text-gray-500`}>Loading…</div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Scheduled Tasks Pro</h1>
          <p className="mt-1 text-sm text-gray-600">Advanced task scheduling with history, templates &amp; monitoring</p>
        </div>
        <div className="flex items-center gap-2">
          <button type="button" onClick={fetchAll} className={BTN_SECONDARY}>
            <RefreshCw size={16} /> Refresh
          </button>
          <button type="button" onClick={() => setShowCreate(true)} className={BTN_PRIMARY}>
            <Plus size={16} /> New Task
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {/* Stats */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          {[
            { label: 'Total Tasks', value: stats.total_tasks ?? 0, icon: <Clock size={20} />, color: 'text-brand-600' },
            { label: 'Enabled', value: stats.enabled_tasks ?? 0, icon: <CheckCircle size={20} />, color: 'text-emerald-600' },
            { label: 'Success Rate', value: `${stats.success_rate ?? 0}%`, icon: <Zap size={20} />, color: 'text-sky-600' },
            { label: 'Failed Today', value: stats.failed_today ?? 0, icon: <XCircle size={20} />, color: 'text-red-600' },
          ].map((stat, i) => (
            <div key={i} className={`${CARD} p-4`}>
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{stat.label}</p>
                  <p className="mt-1 text-2xl font-semibold text-gray-900">{stat.value}</p>
                </div>
                <div className={stat.color}>{stat.icon}</div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="Scheduled task sections">
          {(['tasks', 'history'] as const).map((tab) => (
            <button
              key={tab}
              type="button"
              aria-current={activeTab === tab ? 'page' : undefined}
              onClick={() => setActiveTab(tab)}
              className={`border-b-2 px-1 pb-3 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 ${
                activeTab === tab
                  ? 'border-brand-600 text-brand-700'
                  : 'border-transparent text-gray-600 hover:border-gray-300 hover:text-gray-900'
              }`}
            >
              {tab === 'tasks' ? 'Tasks' : 'Execution History'}
            </button>
          ))}
        </nav>
      </div>

      {/* Tasks Tab */}
      {activeTab === 'tasks' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">All Tasks</h2>
          </div>
          <div>
            {tasks.length === 0 ? (
              <div className="px-5 py-10 text-center text-sm text-gray-500">No tasks configured. Create one to get started.</div>
            ) : (
              tasks.map((task) => {
                const priority = getPriorityLabel(task.priority);
                return (
                  <div key={task.id} className="border-b border-gray-100 px-5 py-4 hover:bg-gray-50">
                    <div className="flex items-start justify-between gap-4">
                      <div className="flex items-start gap-4">
                        <div className={`rounded-md p-2 ${task.is_enabled ? 'bg-emerald-50' : 'bg-gray-100'}`}>
                          {getTypeIcon(task.task_type)}
                        </div>
                        <div>
                          <div className="flex flex-wrap items-center gap-2">
                            <p className="text-sm font-semibold text-gray-900">{task.name}</p>
                            <span className={`${BADGE} ${priority.color}`}>{priority.label}</span>
                            {(task.tags ?? []).map((tag, i) => (
                              <span key={i} className={`${BADGE} bg-gray-100 text-gray-700`}>{tag}</span>
                            ))}
                          </div>
                          <p className="mt-1 text-sm text-gray-600">
                            <span className="font-mono">{task.schedule}</span>
                            {task.schedule_desc && <span className="ml-2">({task.schedule_desc})</span>}
                          </p>
                          <div className="mt-1 flex flex-wrap items-center gap-4 text-xs text-gray-500">
                            <span>Type: {task.task_type}</span>
                            <span>Runs: {task.run_count ?? 0}</span>
                            <span>Fails: {task.fail_count ?? 0}</span>
                            {task.last_run_at && <span suppressHydrationWarning>Last: {formatDateTime(task.last_run_at)}</span>}
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center gap-1">
                        {task.last_status && (
                          <span className={`mr-2 text-xs font-medium ${getStatusColor(task.last_status)}`}>{task.last_status}</span>
                        )}
                        <button
                          type="button"
                          onClick={() => { setSelectedTask(task); fetchTaskExecutions(task.id); }}
                          className={ICON_BTN}
                          aria-label={`View history for ${task.name}`}
                          title="View History"
                        >
                          <History size={16} />
                        </button>
                        <button
                          type="button"
                          onClick={() => handleRunTask(task.id)}
                          className={ICON_BTN}
                          aria-label={`Run ${task.name} now`}
                          title="Run Now"
                        >
                          <Play size={16} />
                        </button>
                        <button
                          type="button"
                          onClick={() => handleToggleTask(task.id)}
                          className={ICON_BTN}
                          aria-label={task.is_enabled ? `Disable ${task.name}` : `Enable ${task.name}`}
                          title={task.is_enabled ? 'Disable' : 'Enable'}
                        >
                          <Pause size={16} />
                        </button>
                        <button
                          type="button"
                          onClick={() => handleDeleteTask(task.id)}
                          className={ICON_DANGER}
                          aria-label={`Delete ${task.name}`}
                          title="Delete"
                        >
                          <Trash2 size={16} />
                        </button>
                      </div>
                    </div>
                  </div>
                );
              })
            )}
          </div>
        </div>
      )}

      {/* History Tab */}
      {activeTab === 'history' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Recent Executions</h2>
          </div>
          <div>
            {executions.length === 0 ? (
              <div className="px-5 py-10 text-center text-sm text-gray-500">No executions yet.</div>
            ) : (
              executions.map((exec) => (
                <div key={exec.id} className="flex items-start justify-between gap-4 border-b border-gray-100 px-5 py-4 hover:bg-gray-50">
                  <div className="flex items-start gap-4">
                    <div className={`rounded-md p-2 ${exec.status === 'success' ? 'bg-emerald-50' : exec.status === 'failed' ? 'bg-red-50' : 'bg-brand-50'}`}>
                      {exec.status === 'success' ? <CheckCircle size={16} className="text-emerald-600" /> :
                       exec.status === 'failed' ? <XCircle size={16} className="text-red-600" /> :
                       <Timer size={16} className="text-brand-600" />}
                    </div>
                    <div>
                      <p className="font-mono text-sm text-gray-900">{exec.output?.substring(0, 80) || 'No output'}</p>
                      <div className="mt-1 flex flex-wrap items-center gap-3 text-xs text-gray-500">
                        <span>By: {exec.triggered_by || '—'}</span>
                        <span>Duration: {exec.duration ?? 0}ms</span>
                        {exec.exit_code !== null && exec.exit_code !== undefined && <span>Exit: {exec.exit_code}</span>}
                      </div>
                    </div>
                  </div>
                  <div className="text-right">
                    <span className={`text-sm font-medium ${getStatusColor(exec.status)}`}>{exec.status || 'unknown'}</span>
                    <p className="mt-1 text-xs text-gray-500" suppressHydrationWarning>{formatDateTime(exec.created_at)}</p>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* Task Execution Detail Modal */}
      {selectedTask && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40 p-4" onClick={() => { setSelectedTask(null); setTaskExecutions([]); }}>
          <div className="max-h-[80vh] w-full max-w-3xl overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg" onClick={(e) => e.stopPropagation()}>
            <div className="flex items-start justify-between gap-4 border-b border-gray-200 px-5 py-4">
              <div>
                <h2 className="text-sm font-semibold text-gray-900">{selectedTask.name}</h2>
                <p className="mt-1 text-sm text-gray-600">Execution History</p>
              </div>
              <button
                type="button"
                aria-label="Close execution history"
                onClick={() => { setSelectedTask(null); setTaskExecutions([]); }}
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X size={18} />
              </button>
            </div>
            <div>
              {taskExecutions.length === 0 ? (
                <div className="px-5 py-10 text-center text-sm text-gray-500">No executions found.</div>
              ) : (
                taskExecutions.map((exec) => (
                  <div key={exec.id} className="border-b border-gray-100 px-5 py-4">
                    <div className="mb-2 flex items-center justify-between">
                      <span className={`text-sm font-medium ${getStatusColor(exec.status)}`}>{exec.status || 'unknown'}</span>
                      <span className="text-xs text-gray-500" suppressHydrationWarning>{formatDateTime(exec.created_at)}</span>
                    </div>
                    {exec.output && <pre className="mt-1 overflow-x-auto rounded-md border border-gray-200 bg-gray-50 p-2 text-xs text-gray-700">{exec.output}</pre>}
                    {exec.error_output && <pre className="mt-1 overflow-x-auto rounded-md border border-red-200 bg-red-50 p-2 text-xs text-red-700">{exec.error_output}</pre>}
                    <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-gray-500">
                      <span>Duration: {exec.duration ?? 0}ms</span>
                      {exec.exit_code !== null && exec.exit_code !== undefined && <span>Exit: {exec.exit_code}</span>}
                      <span>By: {exec.triggered_by || '—'}</span>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      )}

      {/* Create Task Modal */}
      {showCreate && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40 p-4" onClick={() => setShowCreate(false)}>
          <div className="max-h-[80vh] w-full max-w-lg overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg" onClick={(e) => e.stopPropagation()}>
            <div className="border-b border-gray-200 px-5 py-4">
              <h2 className="text-sm font-semibold text-gray-900">Create Scheduled Task</h2>
            </div>
            <div className="space-y-4 p-5">
              <div>
                <label htmlFor="task-name" className={LABEL}>Task Name</label>
                <input id="task-name" type="text" value={newTask.name} onChange={(e) => setNewTask({ ...newTask, name: e.target.value })}
                  className={INPUT}
                  placeholder="e.g., Database Backup" />
              </div>
              <div>
                <label htmlFor="task-description" className={LABEL}>Description</label>
                <input id="task-description" type="text" value={newTask.description} onChange={(e) => setNewTask({ ...newTask, description: e.target.value })}
                  className={INPUT}
                  placeholder="Optional description" />
              </div>
              <div>
                <label htmlFor="task-type" className={LABEL}>Task Type</label>
                <select id="task-type" value={newTask.task_type} onChange={(e) => setNewTask({ ...newTask, task_type: e.target.value })}
                  className={INPUT}>
                  <option value="command">Command</option>
                  <option value="script">Script</option>
                  <option value="http">HTTP Request</option>
                  <option value="backup">Backup</option>
                  <option value="cleanup">Cleanup</option>
                </select>
              </div>
              <div>
                <label htmlFor="task-command" className={LABEL}>Command / Script</label>
                <textarea id="task-command" value={newTask.command} onChange={(e) => setNewTask({ ...newTask, command: e.target.value })}
                  className={`${INPUT} font-mono`}
                  rows={3} placeholder="e.g., /usr/bin/pg_dump mydb > /backups/db.sql" />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label htmlFor="task-schedule" className={LABEL}>Cron Schedule</label>
                  <input id="task-schedule" type="text" value={newTask.schedule} onChange={(e) => setNewTask({ ...newTask, schedule: e.target.value })}
                    className={`${INPUT} font-mono`}
                    placeholder="0 * * * *" />
                </div>
                <div>
                  <label htmlFor="task-priority" className={LABEL}>Priority</label>
                  <select id="task-priority" value={newTask.priority} onChange={(e) => setNewTask({ ...newTask, priority: parseInt(e.target.value) })}
                    className={INPUT}>
                    <option value={1}>Low</option>
                    <option value={2}>Normal</option>
                    <option value={3}>High</option>
                    <option value={4}>Critical</option>
                  </select>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label htmlFor="task-timeout" className={LABEL}>Timeout (seconds, 0=none)</label>
                  <input id="task-timeout" type="number" value={newTask.timeout} onChange={(e) => setNewTask({ ...newTask, timeout: parseInt(e.target.value) || 0 })}
                    className={INPUT} />
                </div>
                <div>
                  <label htmlFor="task-retries" className={LABEL}>Max Retries</label>
                  <input id="task-retries" type="number" value={newTask.max_retries} onChange={(e) => setNewTask({ ...newTask, max_retries: parseInt(e.target.value) || 0 })}
                    className={INPUT} />
                </div>
              </div>
              <div>
                <label htmlFor="task-tags" className={LABEL}>Tags (comma-separated)</label>
                <input id="task-tags" type="text" value={newTask.tags} onChange={(e) => setNewTask({ ...newTask, tags: e.target.value })}
                  className={INPUT}
                  placeholder="backup, database, daily" />
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              <button type="button" onClick={() => setShowCreate(false)} className={BTN_SECONDARY}>Cancel</button>
              <button type="button" onClick={handleCreateTask} disabled={!newTask.name || !newTask.schedule}
                className={BTN_PRIMARY}>Create Task</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
