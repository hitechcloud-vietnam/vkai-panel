'use client';

import { useState, useEffect } from 'react';
import { FileText, Calendar, Send, Plus, Trash2, RefreshCw, Clock, Mail, CheckCircle, XCircle, X } from 'lucide-react';

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
const ICON_DANGER =
  'rounded-md p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500';

function formatDateTime(value?: string | null): string {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString();
}

export default function DailyReportsPage() {
  const [stats, setStats] = useState<ReportStats | null>(null);
  const [reports, setReports] = useState<DailyReport[]>([]);
  const [schedules, setSchedules] = useState<ReportSchedule[]>([]);
  const [deliveries, setDeliveries] = useState<ReportDelivery[]>([]);
  const [activeTab, setActiveTab] = useState<'reports' | 'schedules' | 'deliveries'>('reports');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
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
      const [statsRes, reportsRes, schedulesRes, deliveriesRes] = await Promise.all([
        apiCall('/v1/daily-reports/stats').catch(() => ({ stats: null })),
        apiCall('/v1/daily-reports/reports?limit=30').catch(() => ({ reports: [] })),
        apiCall('/v1/daily-reports/schedules').catch(() => ({ schedules: [] })),
        apiCall('/v1/daily-reports/deliveries?limit=50').catch(() => ({ deliveries: [] })),
      ]);
      setStats(statsRes?.stats ?? null);
      setReports(Array.isArray(reportsRes?.reports) ? reportsRes.reports : []);
      setSchedules(Array.isArray(schedulesRes?.schedules) ? schedulesRes.schedules : []);
      setDeliveries(Array.isArray(deliveriesRes?.deliveries) ? deliveriesRes.deliveries : []);
    } catch (err) {
      console.error('Failed to fetch data:', err);
      setError('Unable to load report data. Please try again.');
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
      setError('Unable to generate the report. Please try again.');
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
      setError('Unable to create the schedule. Please try again.');
    }
  };

  const handleDeleteSchedule = async (id: string) => {
    if (!confirm('Delete this schedule?')) return;
    try {
      await apiCall(`/v1/daily-reports/schedules/${id}`, { method: 'DELETE' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to delete schedule:', err);
      setError('Unable to delete the schedule. Please try again.');
    }
  };

  const handleDeleteReport = async (id: string) => {
    if (!confirm('Delete this report?')) return;
    try {
      await apiCall(`/v1/daily-reports/reports/${id}`, { method: 'DELETE' });
      await fetchAll();
    } catch (err) {
      console.error('Failed to delete report:', err);
      setError('Unable to delete the report. Please try again.');
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
      setError('Unable to update the schedule. Please try again.');
    }
  };

  const fetchReportDetail = async (id: string) => {
    try {
      const res = await apiCall(`/v1/daily-reports/reports/${id}`);
      setSelectedReport(res?.report ?? null);
    } catch (err) {
      console.error('Failed to fetch report:', err);
      setError('Unable to load the report detail. Please try again.');
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'sent': case 'generated': return 'text-emerald-600';
      case 'failed': return 'text-red-600';
      case 'pending': return 'text-amber-600';
      default: return 'text-gray-600';
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
      <div className={`${CARD} px-5 py-12 text-center text-sm text-gray-500`}>Loading…</div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Daily Reports Pro</h1>
          <p className="mt-1 text-sm text-gray-600">Automated server health &amp; status reports</p>
        </div>
        <div className="flex items-center gap-2">
          <button type="button" onClick={fetchAll} className={BTN_SECONDARY}>
            <RefreshCw size={16} />
            Refresh
          </button>
          <button
            type="button"
            onClick={handleGenerateReport}
            disabled={generating}
            className={BTN_PRIMARY}
          >
            <FileText size={16} />
            {generating ? 'Generating...' : 'Generate Report'}
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {/* Stats Cards */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
          {[
            { label: 'Total Reports', value: stats.total_reports ?? 0, icon: <FileText size={20} />, color: 'text-brand-600' },
            { label: 'This Month', value: stats.reports_this_month ?? 0, icon: <Calendar size={20} />, color: 'text-emerald-600' },
            { label: 'Active Schedules', value: stats.active_schedules ?? 0, icon: <Clock size={20} />, color: 'text-sky-600' },
            { label: 'Deliveries', value: stats.total_deliveries ?? 0, icon: <Send size={20} />, color: 'text-gray-600' },
            { label: 'Failed', value: stats.failed_deliveries ?? 0, icon: <XCircle size={20} />, color: 'text-red-600' },
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
        <nav className="-mb-px flex gap-6" aria-label="Daily report sections">
          {(['reports', 'schedules', 'deliveries'] as const).map((tab) => (
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
              {tab.charAt(0).toUpperCase() + tab.slice(1)}
            </button>
          ))}
        </nav>
      </div>

      {/* Reports Tab */}
      {activeTab === 'reports' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Generated Reports</h2>
          </div>
          <div>
            {reports.length === 0 ? (
              <div className="px-5 py-10 text-center text-sm text-gray-500">
                No reports generated yet. Click &quot;Generate Report&quot; to create one.
              </div>
            ) : (
              reports.map((report) => (
                <div
                  key={report.id}
                  className="flex cursor-pointer items-center justify-between gap-4 border-b border-gray-100 px-5 py-4 hover:bg-gray-50"
                  onClick={() => fetchReportDetail(report.id)}
                >
                  <div className="flex items-center gap-4">
                    <div className="rounded-md bg-brand-50 p-2">
                      <FileText size={18} className="text-brand-600" />
                    </div>
                    <div>
                      <p className="text-sm font-semibold text-gray-900">{report.title}</p>
                      <p className="mt-0.5 text-sm text-gray-600">{report.summary}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-4">
                    <div className="text-right">
                      <div className={`flex items-center justify-end gap-1 ${getStatusColor(report.status)}`}>
                        {getStatusIcon(report.status)}
                        <span className="text-sm capitalize">{report.status || 'unknown'}</span>
                      </div>
                      <p className="mt-1 text-xs text-gray-500" suppressHydrationWarning>{formatDateTime(report.created_at)}</p>
                    </div>
                    <button
                      type="button"
                      aria-label={`Delete report ${report.title}`}
                      onClick={(e) => { e.stopPropagation(); handleDeleteReport(report.id); }}
                      className={ICON_DANGER}
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
        <div className={CARD}>
          <div className={`${CARD_HEADER} flex items-center justify-between gap-4`}>
            <h2 className="text-sm font-semibold text-gray-900">Report Schedules</h2>
            <button type="button" onClick={() => setShowCreateSchedule(true)} className={BTN_PRIMARY}>
              <Plus size={14} />
              New Schedule
            </button>
          </div>
          <div>
            {schedules.length === 0 ? (
              <div className="px-5 py-10 text-center text-sm text-gray-500">No schedules configured.</div>
            ) : (
              schedules.map((schedule) => (
                <div key={schedule.id} className="flex items-center justify-between gap-4 border-b border-gray-100 px-5 py-4 hover:bg-gray-50">
                  <div className="flex items-center gap-4">
                    <div className={`rounded-md p-2 ${schedule.is_active ? 'bg-emerald-50' : 'bg-gray-100'}`}>
                      <Clock size={18} className={schedule.is_active ? 'text-emerald-600' : 'text-gray-500'} />
                    </div>
                    <div>
                      <p className="text-sm font-semibold text-gray-900">{schedule.name}</p>
                      <p className="mt-0.5 text-sm text-gray-600">
                        {schedule.frequency} • {schedule.recipients?.length || 0} recipients • {schedule.report_type}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <button
                      type="button"
                      onClick={() => handleToggleSchedule(schedule)}
                      className={`${BADGE} focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 ${
                        schedule.is_active ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-700'
                      }`}
                    >
                      {schedule.is_active ? 'Active' : 'Paused'}
                    </button>
                    <button
                      type="button"
                      aria-label={`Delete schedule ${schedule.name}`}
                      onClick={() => handleDeleteSchedule(schedule.id)}
                      className={ICON_DANGER}
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
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Delivery History</h2>
          </div>
          <div>
            {deliveries.length === 0 ? (
              <div className="px-5 py-10 text-center text-sm text-gray-500">No deliveries yet.</div>
            ) : (
              deliveries.map((delivery) => (
                <div key={delivery.id} className="flex items-center justify-between gap-4 border-b border-gray-100 px-5 py-4 hover:bg-gray-50">
                  <div className="flex items-center gap-4">
                    <Mail size={18} className="text-gray-400" />
                    <div>
                      <p className="text-sm font-medium text-gray-900">{delivery.recipient}</p>
                      <p className="text-xs text-gray-500">via {delivery.channel}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <div className={`flex items-center gap-1 ${getStatusColor(delivery.status)}`}>
                      {getStatusIcon(delivery.status)}
                      <span className="text-sm capitalize">{delivery.status || 'unknown'}</span>
                    </div>
                    {delivery.error && (
                      <span className="max-w-[200px] truncate text-xs text-red-700">{delivery.error}</span>
                    )}
                    <span className="text-xs text-gray-500" suppressHydrationWarning>
                      {formatDateTime(delivery.sent_at)}
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
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40 p-4" onClick={() => setSelectedReport(null)}>
          <div className="max-h-[80vh] w-full max-w-3xl overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg" onClick={(e) => e.stopPropagation()}>
            <div className="flex items-start justify-between gap-4 border-b border-gray-200 px-5 py-4">
              <div>
                <h2 className="text-sm font-semibold text-gray-900">{selectedReport.title}</h2>
                <p className="mt-1 text-sm text-gray-600">{selectedReport.report_date} • {selectedReport.report_type}</p>
              </div>
              <button
                type="button"
                aria-label="Close report detail"
                onClick={() => setSelectedReport(null)}
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X size={18} />
              </button>
            </div>
            <div className="space-y-4 p-5">
              <div className="rounded-md border border-gray-200 bg-gray-50 p-4">
                <p className="text-sm text-gray-700">{selectedReport.summary}</p>
              </div>
              {(selectedReport.sections ?? []).map((section) => (
                <div key={section.id} className="rounded-md border border-gray-200 bg-gray-50 p-4">
                  <h3 className="mb-2 text-sm font-semibold text-gray-900">{section.title}</h3>
                  <p className="text-sm text-gray-700">{section.content}</p>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Create Schedule Modal */}
      {showCreateSchedule && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40 p-4" onClick={() => setShowCreateSchedule(false)}>
          <div className="w-full max-w-lg rounded-lg border border-gray-200 bg-white shadow-lg" onClick={(e) => e.stopPropagation()}>
            <div className="border-b border-gray-200 px-5 py-4">
              <h2 className="text-sm font-semibold text-gray-900">Create Report Schedule</h2>
            </div>
            <div className="space-y-4 p-5">
              <div>
                <label htmlFor="schedule-name" className={LABEL}>Schedule Name</label>
                <input
                  id="schedule-name"
                  type="text"
                  value={newSchedule.name}
                  onChange={(e) => setNewSchedule({ ...newSchedule, name: e.target.value })}
                  className={INPUT}
                  placeholder="e.g., Morning Server Report"
                />
              </div>
              <div>
                <label htmlFor="schedule-type" className={LABEL}>Report Type</label>
                <select
                  id="schedule-type"
                  value={newSchedule.report_type}
                  onChange={(e) => setNewSchedule({ ...newSchedule, report_type: e.target.value })}
                  className={INPUT}
                >
                  <option value="daily">Daily</option>
                  <option value="weekly">Weekly</option>
                  <option value="monthly">Monthly</option>
                </select>
              </div>
              <div>
                <label htmlFor="schedule-cron" className={LABEL}>Cron Schedule</label>
                <input
                  id="schedule-cron"
                  type="text"
                  value={newSchedule.frequency}
                  onChange={(e) => setNewSchedule({ ...newSchedule, frequency: e.target.value })}
                  className={INPUT}
                  placeholder="0 8 * * *"
                />
                <p className="mt-1 text-xs text-gray-500">Default: Daily at 8:00 AM</p>
              </div>
              <div>
                <label htmlFor="schedule-recipients" className={LABEL}>Recipients (comma-separated emails)</label>
                <input
                  id="schedule-recipients"
                  type="text"
                  value={newSchedule.recipients}
                  onChange={(e) => setNewSchedule({ ...newSchedule, recipients: e.target.value })}
                  className={INPUT}
                  placeholder="admin@example.com, ops@example.com"
                />
              </div>
              <div>
                <label htmlFor="schedule-sections" className={LABEL}>Sections (comma-separated)</label>
                <input
                  id="schedule-sections"
                  type="text"
                  value={newSchedule.sections}
                  onChange={(e) => setNewSchedule({ ...newSchedule, sections: e.target.value })}
                  className={INPUT}
                />
              </div>
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4">
              <button type="button" onClick={() => setShowCreateSchedule(false)} className={BTN_SECONDARY}>
                Cancel
              </button>
              <button
                type="button"
                onClick={handleCreateSchedule}
                disabled={!newSchedule.name}
                className={BTN_PRIMARY}
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
