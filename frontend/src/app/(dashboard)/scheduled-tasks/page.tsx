'use client';

import { useState, useEffect } from 'react';
import { Clock, Plus, Play, Pause, Trash2, RefreshCw, CheckCircle, XCircle, Timer, Zap, FileText, Settings, History } from 'lucide-react';

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

export default function ScheduledTasksPage() {
  const [stats, setStats] = useState<TaskStats | null>(null);
  const [tasks, setTasks] = useState<ScheduledTask[]>([]);
  const [executions, setExecutions] = useState<TaskExecution[]>([]);
  const [activeTab, setActiveTab] = useState<'tasks' | 'history'>('tasks');
  const [loading, setLoading] = useState(true);
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
      const authStorage = localStorage.getItem('auth-storage');
      if (authStorage) {
        try {
          const parsed = JSON.parse(authStorage);
          return parsed?.state?.token || '';
        } catch { return ''; }
      }
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
    try {
      const [statsRes, tasksRes, execsRes] = await Promise.all([
        apiCall('/v1/scheduled-tasks/stats').catch(() => ({ stats: null })),
        apiCall('/v1/scheduled-tasks').catch(() => ({ tasks: [] })),
        apiCall('/v1/scheduled-tasks/recent-executions?limit=50').catch(() => ({ executions: [] })),
      ]);
      setStats(statsRes.stats);
      setTasks(tasksRes.tasks || []);
      setExecutions(execsRes.executions || []);
    } catch (err) {
      console.error('Failed to fetch data:', err);
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
    }
  };

  const handleToggleTask = async (id: string) => {
    try {
      await apiCall(`/v1/scheduled-tasks/${id}/toggle`, { method: 'POST' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to toggle task:', err);
    }
  };

  const handleRunTask = async (id: string) => {
    try {
      await apiCall(`/v1/scheduled-tasks/${id}/run`, { method: 'POST' });
      setTimeout(() => fetchAll(), 1000);
    } catch (err) {
      console.error('Failed to run task:', err);
    }
  };

  const handleDeleteTask = async (id: string) => {
    if (!confirm('Delete this task and all its execution history?')) return;
    try {
      await apiCall(`/v1/scheduled-tasks/${id}`, { method: 'DELETE' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to delete task:', err);
    }
  };

  const fetchTaskExecutions = async (taskId: string) => {
    try {
      const res = await apiCall(`/v1/scheduled-tasks/${taskId}/executions?limit=20`);
      setTaskExecutions(res.executions || []);
    } catch (err) {
      console.error('Failed to fetch executions:', err);
    }
  };

  const getPriorityLabel = (p: number) => {
    switch (p) {
      case 1: return { label: 'Low', color: 'text-gray-400' };
      case 2: return { label: 'Normal', color: 'text-blue-400' };
      case 3: return { label: 'High', color: 'text-yellow-400' };
      case 4: return { label: 'Critical', color: 'text-red-400' };
      default: return { label: 'Normal', color: 'text-blue-400' };
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'success': return 'text-green-400';
      case 'failed': return 'text-red-400';
      case 'running': return 'text-blue-400';
      case 'timeout': return 'text-yellow-400';
      default: return 'text-gray-400';
    }
  };

  const getTypeIcon = (type: string) => {
    switch (type) {
      case 'command': return <Zap size={16} />;
      case 'script': return <FileText size={16} />;
      case 'http': return <Settings size={16} />;
      default: return <Clock size={16} />;
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="animate-spin text-blue-400" size={32} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Scheduled Tasks Pro</h1>
          <p className="text-gray-400 mt-1">Advanced task scheduling with history, templates & monitoring</p>
        </div>
        <div className="flex gap-3">
          <button onClick={fetchAll} className="flex items-center gap-2 px-4 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg transition-colors">
            <RefreshCw size={16} /> Refresh
          </button>
          <button onClick={() => setShowCreate(true)} className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors">
            <Plus size={16} /> New Task
          </button>
        </div>
      </div>

      {/* Stats */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          {[
            { label: 'Total Tasks', value: stats.total_tasks, icon: <Clock size={20} />, color: 'text-blue-400' },
            { label: 'Enabled', value: stats.enabled_tasks, icon: <CheckCircle size={20} />, color: 'text-green-400' },
            { label: 'Success Rate', value: `${stats.success_rate}%`, icon: <Zap size={20} />, color: 'text-purple-400' },
            { label: 'Failed Today', value: stats.failed_today, icon: <XCircle size={20} />, color: 'text-red-400' },
          ].map((stat, i) => (
            <div key={i} className="bg-gray-800 rounded-xl p-4 border border-gray-700">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-gray-400 text-sm">{stat.label}</p>
                  <p className={`text-2xl font-bold mt-1 ${stat.color}`}>{stat.value}</p>
                </div>
                <div className={`${stat.color} opacity-60`}>{stat.icon}</div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-1 bg-gray-800 rounded-lg p-1 w-fit">
        {(['tasks', 'history'] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${activeTab === tab ? 'bg-blue-600 text-white' : 'text-gray-400 hover:text-white'}`}
          >
            {tab === 'tasks' ? 'Tasks' : 'Execution History'}
          </button>
        ))}
      </div>

      {/* Tasks Tab */}
      {activeTab === 'tasks' && (
        <div className="bg-gray-800 rounded-xl border border-gray-700">
          <div className="p-4 border-b border-gray-700">
            <h2 className="text-lg font-semibold text-white">All Tasks</h2>
          </div>
          <div className="divide-y divide-gray-700">
            {tasks.length === 0 ? (
              <div className="p-8 text-center text-gray-400">No tasks configured. Create one to get started.</div>
            ) : (
              tasks.map((task) => {
                const priority = getPriorityLabel(task.priority);
                return (
                  <div key={task.id} className="p-4 hover:bg-gray-750">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-4">
                        <div className={`p-2 rounded-lg ${task.is_enabled ? 'bg-green-600/20' : 'bg-gray-600/20'}`}>
                          {getTypeIcon(task.task_type)}
                        </div>
                        <div>
                          <div className="flex items-center gap-2">
                            <p className="text-white font-medium">{task.name}</p>
                            <span className={`text-xs ${priority.color}`}>{priority.label}</span>
                            {task.tags?.map((tag, i) => (
                              <span key={i} className="px-2 py-0.5 bg-gray-700 text-gray-300 text-xs rounded-full">{tag}</span>
                            ))}
                          </div>
                          <p className="text-gray-400 text-sm mt-1">
                            <span className="font-mono">{task.schedule}</span>
                            {task.schedule_desc && <span className="ml-2">({task.schedule_desc})</span>}
                          </p>
                          <div className="flex items-center gap-4 mt-1 text-xs text-gray-500">
                            <span>Type: {task.task_type}</span>
                            <span>Runs: {task.run_count}</span>
                            <span>Fails: {task.fail_count}</span>
                            {task.last_run_at && <span>Last: {new Date(task.last_run_at).toLocaleString()}</span>}
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        {task.last_status && (
                          <span className={`text-xs ${getStatusColor(task.last_status)}`}>{task.last_status}</span>
                        )}
                        <button
                          onClick={() => { setSelectedTask(task); fetchTaskExecutions(task.id); }}
                          className="p-2 text-gray-400 hover:text-blue-400 transition-colors"
                          title="View History"
                        >
                          <History size={16} />
                        </button>
                        <button
                          onClick={() => handleRunTask(task.id)}
                          className="p-2 text-gray-400 hover:text-green-400 transition-colors"
                          title="Run Now"
                        >
                          <Play size={16} />
                        </button>
                        <button
                          onClick={() => handleToggleTask(task.id)}
                          className={`p-2 transition-colors ${task.is_enabled ? 'text-yellow-400 hover:text-yellow-300' : 'text-gray-400 hover:text-green-400'}`}
                          title={task.is_enabled ? 'Disable' : 'Enable'}
                        >
                          <Pause size={16} />
                        </button>
                        <button
                          onClick={() => handleDeleteTask(task.id)}
                          className="p-2 text-gray-400 hover:text-red-400 transition-colors"
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
        <div className="bg-gray-800 rounded-xl border border-gray-700">
          <div className="p-4 border-b border-gray-700">
            <h2 className="text-lg font-semibold text-white">Recent Executions</h2>
          </div>
          <div className="divide-y divide-gray-700">
            {executions.length === 0 ? (
              <div className="p-8 text-center text-gray-400">No executions yet.</div>
            ) : (
              executions.map((exec) => (
                <div key={exec.id} className="p-4 flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <div className={`p-2 rounded-lg ${exec.status === 'success' ? 'bg-green-600/20' : exec.status === 'failed' ? 'bg-red-600/20' : 'bg-blue-600/20'}`}>
                      {exec.status === 'success' ? <CheckCircle size={16} className="text-green-400" /> :
                       exec.status === 'failed' ? <XCircle size={16} className="text-red-400" /> :
                       <Timer size={16} className="text-blue-400" />}
                    </div>
                    <div>
                      <p className="text-white text-sm font-mono">{exec.output?.substring(0, 80) || 'No output'}</p>
                      <div className="flex items-center gap-3 mt-1 text-xs text-gray-500">
                        <span>By: {exec.triggered_by}</span>
                        <span>Duration: {exec.duration}ms</span>
                        {exec.exit_code !== null && <span>Exit: {exec.exit_code}</span>}
                      </div>
                    </div>
                  </div>
                  <div className="text-right">
                    <span className={`text-sm ${getStatusColor(exec.status)}`}>{exec.status}</span>
                    <p className="text-gray-500 text-xs mt-1">{new Date(exec.created_at).toLocaleString()}</p>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* Task Execution Detail Modal */}
      {selectedTask && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => { setSelectedTask(null); setTaskExecutions([]); }}>
          <div className="bg-gray-800 rounded-xl w-full max-w-3xl max-h-[80vh] overflow-y-auto border border-gray-700" onClick={(e) => e.stopPropagation()}>
            <div className="p-6 border-b border-gray-700 flex items-center justify-between">
              <div>
                <h3 className="text-xl font-bold text-white">{selectedTask.name}</h3>
                <p className="text-gray-400 text-sm mt-1">Execution History</p>
              </div>
              <button onClick={() => { setSelectedTask(null); setTaskExecutions([]); }} className="text-gray-400 hover:text-white">✕</button>
            </div>
            <div className="divide-y divide-gray-700">
              {taskExecutions.length === 0 ? (
                <div className="p-8 text-center text-gray-400">No executions found.</div>
              ) : (
                taskExecutions.map((exec) => (
                  <div key={exec.id} className="p-4">
                    <div className="flex items-center justify-between mb-2">
                      <span className={`text-sm font-medium ${getStatusColor(exec.status)}`}>{exec.status}</span>
                      <span className="text-gray-500 text-xs">{new Date(exec.created_at).toLocaleString()}</span>
                    </div>
                    {exec.output && <pre className="text-gray-300 text-xs bg-gray-900 rounded p-2 mt-1 overflow-x-auto">{exec.output}</pre>}
                    {exec.error_output && <pre className="text-red-400 text-xs bg-gray-900 rounded p-2 mt-1 overflow-x-auto">{exec.error_output}</pre>}
                    <div className="flex items-center gap-3 mt-2 text-xs text-gray-500">
                      <span>Duration: {exec.duration}ms</span>
                      {exec.exit_code !== null && <span>Exit: {exec.exit_code}</span>}
                      <span>By: {exec.triggered_by}</span>
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
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => setShowCreate(false)}>
          <div className="bg-gray-800 rounded-xl w-full max-w-lg border border-gray-700 max-h-[80vh] overflow-y-auto" onClick={(e) => e.stopPropagation()}>
            <div className="p-6 border-b border-gray-700">
              <h3 className="text-xl font-bold text-white">Create Scheduled Task</h3>
            </div>
            <div className="p-6 space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Task Name</label>
                <input type="text" value={newTask.name} onChange={(e) => setNewTask({ ...newTask, name: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500"
                  placeholder="e.g., Database Backup" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Description</label>
                <input type="text" value={newTask.description} onChange={(e) => setNewTask({ ...newTask, description: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500"
                  placeholder="Optional description" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Task Type</label>
                <select value={newTask.task_type} onChange={(e) => setNewTask({ ...newTask, task_type: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500">
                  <option value="command">Command</option>
                  <option value="script">Script</option>
                  <option value="http">HTTP Request</option>
                  <option value="backup">Backup</option>
                  <option value="cleanup">Cleanup</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Command / Script</label>
                <textarea value={newTask.command} onChange={(e) => setNewTask({ ...newTask, command: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500 font-mono text-sm"
                  rows={3} placeholder="e.g., /usr/bin/pg_dump mydb > /backups/db.sql" />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1">Cron Schedule</label>
                  <input type="text" value={newTask.schedule} onChange={(e) => setNewTask({ ...newTask, schedule: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500 font-mono"
                    placeholder="0 * * * *" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1">Priority</label>
                  <select value={newTask.priority} onChange={(e) => setNewTask({ ...newTask, priority: parseInt(e.target.value) })}
                    className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500">
                    <option value={1}>Low</option>
                    <option value={2}>Normal</option>
                    <option value={3}>High</option>
                    <option value={4}>Critical</option>
                  </select>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1">Timeout (seconds, 0=none)</label>
                  <input type="number" value={newTask.timeout} onChange={(e) => setNewTask({ ...newTask, timeout: parseInt(e.target.value) || 0 })}
                    className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1">Max Retries</label>
                  <input type="number" value={newTask.max_retries} onChange={(e) => setNewTask({ ...newTask, max_retries: parseInt(e.target.value) || 0 })}
                    className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500" />
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Tags (comma-separated)</label>
                <input type="text" value={newTask.tags} onChange={(e) => setNewTask({ ...newTask, tags: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500"
                  placeholder="backup, database, daily" />
              </div>
            </div>
            <div className="p-6 border-t border-gray-700 flex justify-end gap-3">
              <button onClick={() => setShowCreate(false)} className="px-4 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg transition-colors">Cancel</button>
              <button onClick={handleCreateTask} disabled={!newTask.name || !newTask.schedule}
                className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors disabled:opacity-50">Create Task</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
