'use client';

import { useState, useEffect } from 'react';
import { FileText, Calendar, Send, Plus, Trash2, RefreshCw, Clock, Mail, CheckCircle, XCircle, BarChart3 } from 'lucide-react';

interface DailyReport {
  id: string;
  report_date: string;
  report_type: string;
  title: string;
  summary: string;
  status: string;
  sent_at: string | null;
  created_at: string;
  sections?: ReportSection[];
}

interface ReportSection {
  id: string;
  section_key: string;
  title: string;
  content: string;
  sort_order: number;
}

interface ReportSchedule {
  id: string;
  name: string;
  report_type: string;
  frequency: string;
  recipients: string[];
  sections: string[];
  is_active: boolean;
  last_sent_at: string | null;
  next_send_at: string | null;
  created_at: string;
}

interface ReportDelivery {
  id: string;
  report_id: string;
  recipient: string;
  channel: string;
  status: string;
  error: string | null;
  sent_at: string | null;
  created_at: string;
}

interface ReportStats {
  total_reports: number;
  reports_this_month: number;
  active_schedules: number;
  total_deliveries: number;
  failed_deliveries: number;
  last_report_date: string;
}

export default function DailyReportsPage() {
  const [stats, setStats] = useState<ReportStats | null>(null);
  const [reports, setReports] = useState<DailyReport[]>([]);
  const [schedules, setSchedules] = useState<ReportSchedule[]>([]);
  const [deliveries, setDeliveries] = useState<ReportDelivery[]>([]);
  const [activeTab, setActiveTab] = useState<'reports' | 'schedules' | 'deliveries'>('reports');
  const [loading, setLoading] = useState(true);
  const [showCreateSchedule, setShowCreateSchedule] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [selectedReport, setSelectedReport] = useState<DailyReport | null>(null);
  const [newSchedule, setNewSchedule] = useState({
    name: '',
    report_type: 'daily',
    frequency: '0 8 * * *',
    recipients: '',
    sections: 'server_health,website_status,security,backups',
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
      const [statsRes, reportsRes, schedulesRes, deliveriesRes] = await Promise.all([
        apiCall('/v1/daily-reports/stats').catch(() => ({ stats: null })),
        apiCall('/v1/daily-reports/reports?limit=30').catch(() => ({ reports: [] })),
        apiCall('/v1/daily-reports/schedules').catch(() => ({ schedules: [] })),
        apiCall('/v1/daily-reports/deliveries?limit=50').catch(() => ({ deliveries: [] })),
      ]);
      setStats(statsRes.stats);
      setReports(reportsRes.reports || []);
      setSchedules(schedulesRes.schedules || []);
      setDeliveries(deliveriesRes.deliveries || []);
    } catch (err) {
      console.error('Failed to fetch data:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchAll(); }, []);

  const handleGenerateReport = async () => {
    setGenerating(true);
    try {
      await apiCall('/v1/daily-reports/reports/generate?type=daily', { method: 'POST' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to generate report:', err);
    } finally {
      setGenerating(false);
    }
  };

  const handleCreateSchedule = async () => {
    try {
      await apiCall('/v1/daily-reports/schedules', {
        method: 'POST',
        body: JSON.stringify({
          name: newSchedule.name,
          report_type: newSchedule.report_type,
          frequency: newSchedule.frequency,
          recipients: newSchedule.recipients.split(',').map(s => s.trim()).filter(Boolean),
          sections: newSchedule.sections.split(',').map(s => s.trim()).filter(Boolean),
        }),
      });
      setShowCreateSchedule(false);
      setNewSchedule({ name: '', report_type: 'daily', frequency: '0 8 * * *', recipients: '', sections: 'server_health,website_status,security,backups' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to create schedule:', err);
    }
  };

  const handleDeleteSchedule = async (id: string) => {
    if (!confirm('Delete this schedule?')) return;
    try {
      await apiCall(`/v1/daily-reports/schedules/${id}`, { method: 'DELETE' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to delete schedule:', err);
    }
  };

  const handleDeleteReport = async (id: string) => {
    if (!confirm('Delete this report?')) return;
    try {
      await apiCall(`/v1/daily-reports/reports/${id}`, { method: 'DELETE' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to delete report:', err);
    }
  };

  const handleToggleSchedule = async (schedule: ReportSchedule) => {
    try {
      await apiCall(`/v1/daily-reports/schedules/${schedule.id}`, {
        method: 'PUT',
        body: JSON.stringify({ is_active: !schedule.is_active }),
      });
      await fetchAll();
    } catch (err) {
      console.error('Failed to toggle schedule:', err);
    }
  };

  const fetchReportDetail = async (id: string) => {
    try {
      const res = await apiCall(`/v1/daily-reports/reports/${id}`);
      setSelectedReport(res.report);
    } catch (err) {
      console.error('Failed to fetch report:', err);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'sent': case 'generated': return 'text-green-400';
      case 'failed': return 'text-red-400';
      case 'pending': return 'text-yellow-400';
      default: return 'text-gray-400';
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'sent': case 'generated': return <CheckCircle size={14} />;
      case 'failed': return <XCircle size={14} />;
      default: return <Clock size={14} />;
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
          <h1 className="text-2xl font-bold text-white">Daily Reports Pro</h1>
          <p className="text-gray-400 mt-1">Automated server health & status reports</p>
        </div>
        <div className="flex gap-3">
          <button
            onClick={fetchAll}
            className="flex items-center gap-2 px-4 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg transition-colors"
          >
            <RefreshCw size={16} />
            Refresh
          </button>
          <button
            onClick={handleGenerateReport}
            disabled={generating}
            className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors disabled:opacity-50"
          >
            <FileText size={16} />
            {generating ? 'Generating...' : 'Generate Report'}
          </button>
        </div>
      </div>

      {/* Stats Cards */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
          {[
            { label: 'Total Reports', value: stats.total_reports, icon: <FileText size={20} />, color: 'text-blue-400' },
            { label: 'This Month', value: stats.reports_this_month, icon: <Calendar size={20} />, color: 'text-green-400' },
            { label: 'Active Schedules', value: stats.active_schedules, icon: <Clock size={20} />, color: 'text-purple-400' },
            { label: 'Deliveries', value: stats.total_deliveries, icon: <Send size={20} />, color: 'text-cyan-400' },
            { label: 'Failed', value: stats.failed_deliveries, icon: <XCircle size={20} />, color: 'text-red-400' },
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
        {(['reports', 'schedules', 'deliveries'] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${
              activeTab === tab ? 'bg-blue-600 text-white' : 'text-gray-400 hover:text-white'
            }`}
          >
            {tab.charAt(0).toUpperCase() + tab.slice(1)}
          </button>
        ))}
      </div>

      {/* Reports Tab */}
      {activeTab === 'reports' && (
        <div className="bg-gray-800 rounded-xl border border-gray-700">
          <div className="p-4 border-b border-gray-700">
            <h2 className="text-lg font-semibold text-white">Generated Reports</h2>
          </div>
          <div className="divide-y divide-gray-700">
            {reports.length === 0 ? (
              <div className="p-8 text-center text-gray-400">
                No reports generated yet. Click &quot;Generate Report&quot; to create one.
              </div>
            ) : (
              reports.map((report) => (
                <div
                  key={report.id}
                  className="p-4 hover:bg-gray-750 flex items-center justify-between cursor-pointer"
                  onClick={() => fetchReportDetail(report.id)}
                >
                  <div className="flex items-center gap-4">
                    <div className="p-2 bg-blue-600/20 rounded-lg">
                      <FileText size={20} className="text-blue-400" />
                    </div>
                    <div>
                      <p className="text-white font-medium">{report.title}</p>
                      <p className="text-gray-400 text-sm mt-1">{report.summary}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-4">
                    <div className="text-right">
                      <div className={`flex items-center gap-1 ${getStatusColor(report.status)}`}>
                        {getStatusIcon(report.status)}
                        <span className="text-sm capitalize">{report.status}</span>
                      </div>
                      <p className="text-gray-500 text-xs mt-1">{new Date(report.created_at).toLocaleString()}</p>
                    </div>
                    <button
                      onClick={(e) => { e.stopPropagation(); handleDeleteReport(report.id); }}
                      className="p-2 text-gray-400 hover:text-red-400 transition-colors"
                    >
                      <Trash2 size={16} />
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* Schedules Tab */}
      {activeTab === 'schedules' && (
        <div className="bg-gray-800 rounded-xl border border-gray-700">
          <div className="p-4 border-b border-gray-700 flex items-center justify-between">
            <h2 className="text-lg font-semibold text-white">Report Schedules</h2>
            <button
              onClick={() => setShowCreateSchedule(true)}
              className="flex items-center gap-2 px-3 py-1.5 bg-blue-600 hover:bg-blue-700 text-white rounded-lg text-sm transition-colors"
            >
              <Plus size={14} />
              New Schedule
            </button>
          </div>
          <div className="divide-y divide-gray-700">
            {schedules.length === 0 ? (
              <div className="p-8 text-center text-gray-400">No schedules configured.</div>
            ) : (
              schedules.map((schedule) => (
                <div key={schedule.id} className="p-4 flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <div className={`p-2 rounded-lg ${schedule.is_active ? 'bg-green-600/20' : 'bg-gray-600/20'}`}>
                      <Clock size={20} className={schedule.is_active ? 'text-green-400' : 'text-gray-400'} />
                    </div>
                    <div>
                      <p className="text-white font-medium">{schedule.name}</p>
                      <p className="text-gray-400 text-sm">
                        {schedule.frequency} • {schedule.recipients?.length || 0} recipients • {schedule.report_type}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <button
                      onClick={() => handleToggleSchedule(schedule)}
                      className={`px-3 py-1 rounded-full text-xs font-medium ${
                        schedule.is_active ? 'bg-green-600/20 text-green-400' : 'bg-gray-600/20 text-gray-400'
                      }`}
                    >
                      {schedule.is_active ? 'Active' : 'Paused'}
                    </button>
                    <button
                      onClick={() => handleDeleteSchedule(schedule.id)}
                      className="p-2 text-gray-400 hover:text-red-400 transition-colors"
                    >
                      <Trash2 size={16} />
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* Deliveries Tab */}
      {activeTab === 'deliveries' && (
        <div className="bg-gray-800 rounded-xl border border-gray-700">
          <div className="p-4 border-b border-gray-700">
            <h2 className="text-lg font-semibold text-white">Delivery History</h2>
          </div>
          <div className="divide-y divide-gray-700">
            {deliveries.length === 0 ? (
              <div className="p-8 text-center text-gray-400">No deliveries yet.</div>
            ) : (
              deliveries.map((delivery) => (
                <div key={delivery.id} className="p-4 flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <Mail size={18} className="text-gray-400" />
                    <div>
                      <p className="text-white text-sm">{delivery.recipient}</p>
                      <p className="text-gray-500 text-xs">via {delivery.channel}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <div className={`flex items-center gap-1 ${getStatusColor(delivery.status)}`}>
                      {getStatusIcon(delivery.status)}
                      <span className="text-sm capitalize">{delivery.status}</span>
                    </div>
                    {delivery.error && (
                      <span className="text-red-400 text-xs max-w-[200px] truncate">{delivery.error}</span>
                    )}
                    <span className="text-gray-500 text-xs">
                      {delivery.sent_at ? new Date(delivery.sent_at).toLocaleString() : '—'}
                    </span>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* Report Detail Modal */}
      {selectedReport && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => setSelectedReport(null)}>
          <div className="bg-gray-800 rounded-xl w-full max-w-3xl max-h-[80vh] overflow-y-auto border border-gray-700" onClick={(e) => e.stopPropagation()}>
            <div className="p-6 border-b border-gray-700 flex items-center justify-between">
              <div>
                <h3 className="text-xl font-bold text-white">{selectedReport.title}</h3>
                <p className="text-gray-400 text-sm mt-1">{selectedReport.report_date} • {selectedReport.report_type}</p>
              </div>
              <button onClick={() => setSelectedReport(null)} className="text-gray-400 hover:text-white">✕</button>
            </div>
            <div className="p-6 space-y-4">
              <div className="bg-gray-750 rounded-lg p-4">
                <p className="text-gray-300">{selectedReport.summary}</p>
              </div>
              {selectedReport.sections?.map((section) => (
                <div key={section.id} className="bg-gray-750 rounded-lg p-4">
                  <h4 className="text-white font-medium mb-2">{section.title}</h4>
                  <p className="text-gray-300 text-sm">{section.content}</p>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Create Schedule Modal */}
      {showCreateSchedule && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => setShowCreateSchedule(false)}>
          <div className="bg-gray-800 rounded-xl w-full max-w-lg border border-gray-700" onClick={(e) => e.stopPropagation()}>
            <div className="p-6 border-b border-gray-700">
              <h3 className="text-xl font-bold text-white">Create Report Schedule</h3>
            </div>
            <div className="p-6 space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Schedule Name</label>
                <input
                  type="text"
                  value={newSchedule.name}
                  onChange={(e) => setNewSchedule({ ...newSchedule, name: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500"
                  placeholder="e.g., Morning Server Report"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Report Type</label>
                <select
                  value={newSchedule.report_type}
                  onChange={(e) => setNewSchedule({ ...newSchedule, report_type: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500"
                >
                  <option value="daily">Daily</option>
                  <option value="weekly">Weekly</option>
                  <option value="monthly">Monthly</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Cron Schedule</label>
                <input
                  type="text"
                  value={newSchedule.frequency}
                  onChange={(e) => setNewSchedule({ ...newSchedule, frequency: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500"
                  placeholder="0 8 * * *"
                />
                <p className="text-gray-500 text-xs mt-1">Default: Daily at 8:00 AM</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Recipients (comma-separated emails)</label>
                <input
                  type="text"
                  value={newSchedule.recipients}
                  onChange={(e) => setNewSchedule({ ...newSchedule, recipients: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500"
                  placeholder="admin@example.com, ops@example.com"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Sections (comma-separated)</label>
                <input
                  type="text"
                  value={newSchedule.sections}
                  onChange={(e) => setNewSchedule({ ...newSchedule, sections: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-white focus:outline-none focus:border-blue-500"
                />
              </div>
            </div>
            <div className="p-6 border-t border-gray-700 flex justify-end gap-3">
              <button
                onClick={() => setShowCreateSchedule(false)}
                className="px-4 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleCreateSchedule}
                disabled={!newSchedule.name}
                className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors disabled:opacity-50"
              >
                Create Schedule
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
