'use client';

import { useState, useEffect } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Activity, Bell, Settings, Plus, RefreshCw } from 'lucide-react';
import { monitoringApi } from '@/services/api';

interface Metric {
  id: string;
  server_id: string;
  metric_type: string;
  value: number;
  unit: string;
  recorded_at: string;
}

interface Alert {
  id: string;
  name: string;
  metric_type: string;
  condition: string;
  threshold: number;
  status: string;
  enabled: boolean;
}

interface Dashboard {
  id: string;
  name: string;
  description: string;
  is_default: boolean;
}

const CARD_CLASS = 'rounded-lg border border-gray-200 bg-white shadow-sm';
const CARD_HEADER_CLASS =
  'flex flex-row items-center justify-between space-y-0 border-b border-gray-200 px-5 py-4';
const CARD_TITLE_CLASS = 'text-sm font-semibold text-gray-900';
const BTN_PRIMARY = 'bg-brand-600 text-white hover:bg-brand-700 focus-visible:ring-brand-500';
const BTN_SECONDARY =
  'border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus-visible:ring-brand-500';
const BADGE_BASE = 'rounded-md border-transparent px-2 py-0.5 text-xs font-medium';
const TABS_LIST_CLASS =
  'inline-flex h-auto items-center gap-1 rounded-md border border-gray-200 bg-white p-1 text-gray-600';
const TABS_TRIGGER_CLASS =
  'rounded-md px-3 py-1.5 text-sm font-medium text-gray-600 data-[state=active]:bg-brand-50 data-[state=active]:text-brand-700 data-[state=active]:shadow-none';

function formatTimestamp(value: string): string {
  if (!value) return '—';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return '—';
  return parsed.toLocaleString();
}

function shortId(value: string | undefined | null, length = 8): string {
  if (!value) return '—';
  return value.length > length ? `${value.slice(0, length)}...` : value;
}

function formatValue(value: number | undefined | null): string {
  return typeof value === 'number' && Number.isFinite(value) ? value.toFixed(2) : '0.00';
}

export default function MonitoringPage() {
  const [metrics, setMetrics] = useState<Metric[]>([]);
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [dashboards, setDashboards] = useState<Dashboard[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState('overview');

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);
      const [metricsRes, alertsRes, dashboardsRes] = await Promise.all([
        monitoringApi.getMetrics({ limit: 100 }),
        monitoringApi.getAlerts(),
        monitoringApi.getDashboards(),
      ]);
      setMetrics(Array.isArray(metricsRes?.data?.metrics) ? metricsRes.data.metrics : []);
      setAlerts(Array.isArray(alertsRes?.data?.alerts) ? alertsRes.data.alerts : []);
      setDashboards(Array.isArray(dashboardsRes?.data?.dashboards) ? dashboardsRes.data.dashboards : []);
    } catch (err: any) {
      console.error('Failed to fetch monitoring data:', err);
      setMetrics([]);
      setAlerts([]);
      setDashboards([]);
      setError(err?.response?.data?.message || 'Failed to load monitoring data');
    } finally {
      setLoading(false);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'ok':
      case 'active':
        return 'bg-emerald-50 text-emerald-700';
      case 'warning':
        return 'bg-amber-50 text-amber-700';
      case 'critical':
      case 'firing':
        return 'bg-red-50 text-red-700';
      default:
        return 'bg-gray-100 text-gray-700';
    }
  };

  const getMetricTypeLabel = (type: string) => {
    const labels: Record<string, string> = {
      cpu_usage: 'CPU Usage',
      memory_usage: 'Memory Usage',
      disk_usage: 'Disk Usage',
      network_in: 'Network In',
      network_out: 'Network Out',
      load_average: 'Load Average',
    };
    return labels[type] || type || '—';
  };

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <RefreshCw className="h-6 w-6 animate-spin text-gray-400" aria-hidden="true" />
        <span className="ml-2 text-sm text-gray-600">Loading monitoring data...</span>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Monitoring</h1>
          <p className="mt-1 text-sm text-gray-600">System metrics, alerts, and dashboards</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="outline" onClick={fetchData} className={BTN_SECONDARY}>
            <RefreshCw className="mr-2 h-4 w-4" aria-hidden="true" />
            Refresh
          </Button>
          <Button className={BTN_PRIMARY}>
            <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
            Add Alert
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
          {error}
        </div>
      )}

      <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-4">
        <TabsList className={TABS_LIST_CLASS}>
          <TabsTrigger value="overview" className={TABS_TRIGGER_CLASS}>Overview</TabsTrigger>
          <TabsTrigger value="metrics" className={TABS_TRIGGER_CLASS}>Metrics</TabsTrigger>
          <TabsTrigger value="alerts" className={TABS_TRIGGER_CLASS}>Alerts</TabsTrigger>
          <TabsTrigger value="dashboards" className={TABS_TRIGGER_CLASS}>Dashboards</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Card className={CARD_CLASS}>
              <CardHeader className={CARD_HEADER_CLASS}>
                <CardTitle className={CARD_TITLE_CLASS}>Active Alerts</CardTitle>
                <Bell className="h-4 w-4 text-gray-400" aria-hidden="true" />
              </CardHeader>
              <CardContent className="px-5 py-4">
                <div className="text-2xl font-semibold text-gray-900">
                  {alerts.filter((a) => a?.status === 'firing').length}
                </div>
                <p className="mt-1 text-xs text-gray-500">
                  {alerts.filter((a) => a?.status === 'ok').length} resolved
                </p>
              </CardContent>
            </Card>

            <Card className={CARD_CLASS}>
              <CardHeader className={CARD_HEADER_CLASS}>
                <CardTitle className={CARD_TITLE_CLASS}>Dashboards</CardTitle>
                <Activity className="h-4 w-4 text-gray-400" aria-hidden="true" />
              </CardHeader>
              <CardContent className="px-5 py-4">
                <div className="text-2xl font-semibold text-gray-900">{dashboards.length}</div>
                <p className="mt-1 text-xs text-gray-500">
                  {dashboards.filter((d) => d?.is_default).length} default
                </p>
              </CardContent>
            </Card>

            <Card className={CARD_CLASS}>
              <CardHeader className={CARD_HEADER_CLASS}>
                <CardTitle className={CARD_TITLE_CLASS}>Metrics Collected</CardTitle>
                <Settings className="h-4 w-4 text-gray-400" aria-hidden="true" />
              </CardHeader>
              <CardContent className="px-5 py-4">
                <div className="text-2xl font-semibold text-gray-900">{metrics.length}</div>
                <p className="mt-1 text-xs text-gray-500">Last 24 hours</p>
              </CardContent>
            </Card>
          </div>

          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Recent Metrics</CardTitle>
            </CardHeader>
            <CardContent className="px-5 py-4">
              {metrics.length === 0 ? (
                <div className="py-8 text-center">
                  <Activity className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                  <p className="mt-3 text-sm font-semibold text-gray-900">No metrics collected</p>
                  <p className="mt-1 text-sm text-gray-600">
                    Metrics will appear once agents start reporting.
                  </p>
                </div>
              ) : (
                <div className="divide-y divide-gray-200">
                  {metrics.slice(0, 10).map((metric) => (
                    <div key={metric.id} className="flex items-center justify-between py-3">
                      <div>
                        <p className="text-sm font-medium text-gray-900">
                          {getMetricTypeLabel(metric.metric_type)}
                        </p>
                        <p className="mt-0.5 text-xs text-gray-500">
                          Server: <span className="font-mono">{shortId(metric.server_id)}</span>
                        </p>
                      </div>
                      <div className="text-right">
                        <p className="text-sm font-semibold text-gray-900">
                          {formatValue(metric.value)} {metric.unit || ''}
                        </p>
                        <p className="mt-0.5 text-xs text-gray-500" suppressHydrationWarning>
                          {formatTimestamp(metric.recorded_at)}
                        </p>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="metrics" className="space-y-4">
          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>All Metrics</CardTitle>
              <span className="text-xs text-gray-500">{metrics.length} records</span>
            </CardHeader>
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr className="[&_th]:border-b [&_th]:border-gray-200">
                      <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Type</th>
                      <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Server</th>
                      <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Value</th>
                      <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Time</th>
                    </tr>
                  </thead>
                  <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                    {metrics.length === 0 ? (
                      <tr>
                        <td colSpan={4} className="px-4 py-12 text-center text-sm text-gray-600">
                          No metrics recorded yet
                        </td>
                      </tr>
                    ) : (
                      metrics.map((metric) => (
                        <tr key={metric.id} className="hover:bg-gray-50">
                          <td className="px-4 py-3 text-sm text-gray-700">
                            {getMetricTypeLabel(metric.metric_type)}
                          </td>
                          <td className="px-4 py-3 font-mono text-sm text-gray-700">
                            {shortId(metric.server_id)}
                          </td>
                          <td className="px-4 py-3 text-sm font-medium text-gray-900">
                            {formatValue(metric.value)} {metric.unit || ''}
                          </td>
                          <td className="px-4 py-3 text-sm text-gray-600" suppressHydrationWarning>
                            {formatTimestamp(metric.recorded_at)}
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="alerts" className="space-y-4">
          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Alert Rules</CardTitle>
              <span className="text-xs text-gray-500">{alerts.length} rules</span>
            </CardHeader>
            <CardContent className="px-5 py-4">
              {alerts.length === 0 ? (
                <div className="py-8 text-center">
                  <Bell className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                  <p className="mt-3 text-sm font-semibold text-gray-900">No alert rules</p>
                  <p className="mt-1 text-sm text-gray-600">Create an alert rule to get notified.</p>
                </div>
              ) : (
                <div className="space-y-3">
                  {alerts.map((alert) => (
                    <div
                      key={alert.id}
                      className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-gray-200 px-4 py-3"
                    >
                      <div>
                        <p className="text-sm font-medium text-gray-900">{alert.name || '—'}</p>
                        <p className="mt-0.5 text-xs text-gray-500">
                          {getMetricTypeLabel(alert.metric_type)} {alert.condition || ''} {alert.threshold ?? ''}
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        <Badge variant="outline" className={`${BADGE_BASE} ${getStatusColor(alert.status)}`}>
                          {alert.status || 'unknown'}
                        </Badge>
                        <Badge variant="outline"
                          className={`${BADGE_BASE} ${
                            alert.enabled ? 'bg-brand-50 text-brand-700' : 'bg-gray-100 text-gray-700'
                          }`}
                        >
                          {alert.enabled ? 'Enabled' : 'Disabled'}
                        </Badge>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="dashboards" className="space-y-4">
          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Dashboards</CardTitle>
              <span className="text-xs text-gray-500">{dashboards.length} dashboards</span>
            </CardHeader>
            <CardContent className="px-5 py-4">
              {dashboards.length === 0 ? (
                <div className="py-8 text-center">
                  <Activity className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                  <p className="mt-3 text-sm font-semibold text-gray-900">No dashboards</p>
                  <p className="mt-1 text-sm text-gray-600">Create a dashboard to visualise metrics.</p>
                </div>
              ) : (
                <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
                  {dashboards.map((dashboard) => (
                    <div key={dashboard.id} className="rounded-md border border-gray-200 p-4">
                      <p className="text-sm font-semibold text-gray-900">{dashboard.name || '—'}</p>
                      <p className="mt-1 text-sm text-gray-600">
                        {dashboard.description || 'No description'}
                      </p>
                      {dashboard.is_default && (
                        <Badge variant="outline" className={`${BADGE_BASE} mt-3 bg-brand-50 text-brand-700`}>Default</Badge>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
