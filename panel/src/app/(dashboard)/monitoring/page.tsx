'use client';

/**
 * /monitoring
 *
 * This page used to call GET /api/v1/monitoring/metrics, which is not mounted:
 * every load ended in "Failed to load monitoring data". The metric series is
 * per node - GET /api/v1/monitoring/servers/:server_id/metrics - and it was
 * written before the panel had any servers to name. The panel host is now
 * registered as a node, so there is always a server id available and the call
 * is made against the selected one, defaulting to the panel host and showing a
 * picker only when there is more than one node, which is the pattern the rest
 * of the panel follows.
 *
 * Every endpoint this page calls was checked against
 * core/internal/handler/router.go:
 *
 *   GET    /api/v1/servers                                  router.go:341
 *   GET    /api/v1/servers/:id/metrics                      router.go:345
 *   GET    /api/v1/monitoring/servers/:server_id/metrics    router.go:553
 *   GET    /api/v1/monitoring/alerts                        router.go:557
 *   POST   /api/v1/monitoring/alerts                        router.go:556
 *   PUT    /api/v1/monitoring/alerts/:id                    router.go:559
 *   DELETE /api/v1/monitoring/alerts/:id                    router.go:560
 *   GET    /api/v1/monitoring/dashboards                    router.go:564
 *
 * Two shapes matter and differ. The monitoring handler answers with c.JSON, so
 * its body is {"metrics": [...]} with no envelope, while the server handler
 * answers through utils.Success, so its body is {"success": true, "data": ...}.
 * Reading one as the other is what produced empty lists elsewhere in this
 * codebase, so each call unwraps the shape its own handler sends.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Activity, AlertTriangle, Bell, Plus, RefreshCw, Trash2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import ServerScopeField from '@/components/servers/ServerScopeField';
import AlertFormDialog from '@/components/monitoring/AlertFormDialog';
import useServers from '@/hooks/useServers';
import { monitoringApi, serverApi, unwrap } from '@/services/api';
import { errorMessage } from '@/lib/apiError';
import {
  finiteOrNull,
  formatUsage,
  serverLabel,
  UNAVAILABLE,
} from '@/lib/servers';
import type { ServerMetrics } from '@/types/server';

/** One row of monitoring_metrics. Mirrors models.MonitoringMetric exactly. */
interface MetricSample {
  id: string;
  server_id: string;
  /** The column is `metric`, not `metric_type`. */
  metric: string;
  value: number;
  unit: string;
  /** The column is `timestamp`, not `recorded_at`. */
  timestamp: string;
}

/** Mirrors models.MonitoringAlert. The enabled flag is `is_active`. */
interface AlertRule {
  id: string;
  name: string;
  description: string;
  metric: string;
  condition: string;
  threshold: number;
  severity: string;
  status: string;
  is_active: boolean;
  server_id: string | null;
  last_triggered_at: string | null;
}

interface DashboardRow {
  id: string;
  name: string;
  description: string;
  is_default: boolean;
}

const CARD_CLASS = 'rounded-lg border border-gray-200 bg-white shadow-sm';
const CARD_HEADER_CLASS =
  'flex flex-row items-center justify-between space-y-0 border-b border-gray-200 px-5 py-4';
const CARD_TITLE_CLASS = 'text-sm font-semibold text-gray-900';
const BADGE_BASE = 'rounded-md border-transparent px-2 py-0.5 text-xs font-medium';
const TABS_LIST_CLASS =
  'inline-flex h-auto items-center gap-1 rounded-md border border-gray-200 bg-white p-1 text-gray-600';
const TABS_TRIGGER_CLASS =
  'rounded-md px-3 py-1.5 text-sm font-medium text-gray-600 data-[state=active]:bg-brand-50 data-[state=active]:text-brand-700 data-[state=active]:shadow-none';
const TH_CLASS = 'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
const SELECT_CLASS =
  'h-9 rounded-md border border-gray-300 bg-white px-3 text-sm text-gray-900 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500';

/**
 * Metric names offered in the picker. The column is free text - whatever a
 * caller POSTs is what is stored - so these are suggestions, not a schema, and
 * the names of metrics that existing alert rules already watch are added to
 * them.
 */
const SUGGESTED_METRICS = [
  'cpu_usage',
  'memory_usage',
  'disk_usage',
  'load_average',
  'network_in',
  'network_out',
];

const RANGES: { value: string; label: string; hours: number }[] = [
  { value: '1h', label: 'Last hour', hours: 1 },
  { value: '6h', label: 'Last 6 hours', hours: 6 },
  { value: '24h', label: 'Last 24 hours', hours: 24 },
  { value: '7d', label: 'Last 7 days', hours: 24 * 7 },
];

/** RFC3339 without fractional seconds, which is what the handler parses. */
function rfc3339(date: Date): string {
  return date.toISOString().replace(/\.\d{3}Z$/, 'Z');
}

function formatTimestamp(value: string | null | undefined): string {
  if (!value) return UNAVAILABLE;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return UNAVAILABLE;
  return parsed.toLocaleString();
}

/** A figure the API did not send stays unavailable rather than becoming zero. */
function formatNumber(value: unknown, digits = 2): string {
  const num = finiteOrNull(value);
  return num === null ? UNAVAILABLE : num.toFixed(digits);
}

function statusBadgeClass(status: string): string {
  switch (String(status || '').trim().toLowerCase()) {
    case 'ok':
    case 'active':
      return 'bg-emerald-50 text-emerald-700';
    case 'warning':
      return 'bg-amber-50 text-amber-700';
    case 'critical':
    case 'firing':
    case 'triggered':
      return 'bg-red-50 text-red-700';
    default:
      return 'bg-gray-100 text-gray-700';
  }
}

function severityBadgeClass(severity: string): string {
  switch (String(severity || '').trim().toLowerCase()) {
    case 'critical':
      return 'bg-red-50 text-red-700';
    case 'warning':
      return 'bg-amber-50 text-amber-700';
    case 'info':
      return 'bg-sky-50 text-sky-700';
    default:
      return 'bg-gray-100 text-gray-700';
  }
}

export default function MonitoringPage() {
  const { servers, defaultId, loading: serversLoading, error: serversError } = useServers();

  const [serverId, setServerId] = useState('');
  const [metric, setMetric] = useState(SUGGESTED_METRICS[0]);
  const [range, setRange] = useState('24h');
  const [activeTab, setActiveTab] = useState('overview');

  const [samples, setSamples] = useState<MetricSample[]>([]);
  const [samplesLoading, setSamplesLoading] = useState(false);
  const [samplesError, setSamplesError] = useState<string | null>(null);

  const [nodeMetrics, setNodeMetrics] = useState<ServerMetrics | null>(null);
  const [nodeMetricsError, setNodeMetricsError] = useState<string | null>(null);

  const [alerts, setAlerts] = useState<AlertRule[]>([]);
  const [alertsError, setAlertsError] = useState<string | null>(null);
  const [dashboards, setDashboards] = useState<DashboardRow[]>([]);
  const [dashboardsError, setDashboardsError] = useState<string | null>(null);

  const [alertBusyId, setAlertBusyId] = useState<string | null>(null);
  const [alertActionError, setAlertActionError] = useState<string | null>(null);
  const [alertDialogOpen, setAlertDialogOpen] = useState(false);

  // The panel host when it is the only node, the operator's choice otherwise.
  useEffect(() => {
    if (!serverId && defaultId) setServerId(defaultId);
  }, [defaultId, serverId]);

  const selectedServer = useMemo(
    () => servers.find((server) => server.id === serverId) || null,
    [servers, serverId]
  );

  const loadAlerts = useCallback(async () => {
    setAlertsError(null);
    try {
      // The monitoring handler answers with c.JSON, so the list is at the top
      // level of the body rather than inside a `data` envelope.
      const res = await monitoringApi.getAlerts();
      const rows = res?.data?.alerts;
      setAlerts(Array.isArray(rows) ? rows : []);
    } catch (err) {
      setAlerts([]);
      setAlertsError(errorMessage(err, 'Alert rules could not be loaded.'));
    }
  }, []);

  const loadDashboards = useCallback(async () => {
    setDashboardsError(null);
    try {
      const res = await monitoringApi.getDashboards();
      const rows = res?.data?.dashboards;
      setDashboards(Array.isArray(rows) ? rows : []);
    } catch (err) {
      setDashboards([]);
      setDashboardsError(errorMessage(err, 'Dashboards could not be loaded.'));
    }
  }, []);

  const loadSamples = useCallback(async () => {
    if (!serverId || !metric) {
      setSamples([]);
      return;
    }
    setSamplesLoading(true);
    setSamplesError(null);
    try {
      const hours = RANGES.find((option) => option.value === range)?.hours ?? 24;
      const end = new Date();
      const start = new Date(end.getTime() - hours * 60 * 60 * 1000);
      const res = await monitoringApi.getServerMetrics(serverId, {
        // `metric` is required by the handler; without it the request is a 400.
        metric,
        start: rfc3339(start),
        end: rfc3339(end),
        limit: 1000,
      });
      const rows = res?.data?.metrics;
      setSamples(Array.isArray(rows) ? rows : []);
    } catch (err) {
      setSamples([]);
      setSamplesError(errorMessage(err, 'Metrics could not be loaded for this node.'));
    } finally {
      setSamplesLoading(false);
    }
  }, [serverId, metric, range]);

  const loadNodeMetrics = useCallback(async () => {
    if (!serverId) {
      setNodeMetrics(null);
      return;
    }
    setNodeMetricsError(null);
    try {
      // This one goes through utils.Success, so the payload is under `data`.
      const res = await serverApi.metrics(serverId);
      setNodeMetrics(unwrap<ServerMetrics>(res, null));
    } catch (err) {
      setNodeMetrics(null);
      setNodeMetricsError(
        errorMessage(err, 'No agent sample has been reported for this node yet.')
      );
    }
  }, [serverId]);

  useEffect(() => {
    void loadAlerts();
    void loadDashboards();
  }, [loadAlerts, loadDashboards]);

  useEffect(() => {
    void loadSamples();
  }, [loadSamples]);

  useEffect(() => {
    void loadNodeMetrics();
  }, [loadNodeMetrics]);

  const refreshAll = useCallback(() => {
    void loadAlerts();
    void loadDashboards();
    void loadSamples();
    void loadNodeMetrics();
  }, [loadAlerts, loadDashboards, loadSamples, loadNodeMetrics]);

  const metricOptions = useMemo(() => {
    const fromAlerts = alerts.map((alert) => String(alert.metric || '').trim()).filter(Boolean);
    return Array.from(new Set([...SUGGESTED_METRICS, ...fromAlerts]));
  }, [alerts]);

  const toggleAlert = useCallback(
    async (alert: AlertRule) => {
      setAlertBusyId(alert.id);
      setAlertActionError(null);
      try {
        // PUT is a partial update on the server, so sending only is_active
        // leaves every other field alone.
        await monitoringApi.updateAlert(alert.id, { is_active: !alert.is_active });
        await loadAlerts();
      } catch (err) {
        setAlertActionError(errorMessage(err, 'The rule could not be updated.'));
      } finally {
        setAlertBusyId(null);
      }
    },
    [loadAlerts]
  );

  const deleteAlert = useCallback(
    async (alert: AlertRule) => {
      setAlertBusyId(alert.id);
      setAlertActionError(null);
      try {
        await monitoringApi.deleteAlert(alert.id);
        await loadAlerts();
      } catch (err) {
        setAlertActionError(errorMessage(err, 'The rule could not be deleted.'));
      } finally {
        setAlertBusyId(null);
      }
    },
    [loadAlerts]
  );

  const firing = alerts.filter((alert) =>
    ['triggered', 'firing', 'critical'].includes(String(alert.status || '').toLowerCase())
  ).length;

  const nodeName = selectedServer ? serverLabel(selectedServer) : UNAVAILABLE;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Monitoring</h1>
          <p className="mt-1 text-sm text-gray-600">
            Recorded metrics, alert rules and dashboards for one node at a time.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button type="button" variant="secondary" onClick={refreshAll}>
            <RefreshCw size={16} aria-hidden="true" />
            Refresh
          </Button>
          <Button type="button" onClick={() => setAlertDialogOpen(true)}>
            <Plus size={16} aria-hidden="true" />
            New alert rule
          </Button>
        </div>
      </div>

      {serversError ? (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
          {serversError}
        </div>
      ) : null}

      <div className={`${CARD_CLASS} px-5 py-4`}>
        <ServerScopeField
          id="monitoring-server"
          servers={servers}
          value={serverId}
          onChange={setServerId}
          className="max-w-md"
        />
        {serversLoading ? (
          <p className="mt-2 text-sm text-gray-600">Reading the node list...</p>
        ) : null}
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-4">
        <TabsList className={TABS_LIST_CLASS}>
          <TabsTrigger value="overview" className={TABS_TRIGGER_CLASS}>Overview</TabsTrigger>
          <TabsTrigger value="metrics" className={TABS_TRIGGER_CLASS}>Metrics</TabsTrigger>
          <TabsTrigger value="alerts" className={TABS_TRIGGER_CLASS}>Alerts</TabsTrigger>
          <TabsTrigger value="dashboards" className={TABS_TRIGGER_CLASS}>Dashboards</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
            <Card className={CARD_CLASS}>
              <CardHeader className={CARD_HEADER_CLASS}>
                <CardTitle className={CARD_TITLE_CLASS}>Alert rules firing</CardTitle>
                <Bell className="h-4 w-4 text-gray-400" aria-hidden="true" />
              </CardHeader>
              <CardContent className="px-5 py-4">
                <div className="text-2xl font-semibold text-gray-900">{firing}</div>
                <p className="mt-1 text-xs text-gray-500">
                  {alerts.length} rule{alerts.length === 1 ? '' : 's'} in total
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
                  {dashboards.filter((row) => row?.is_default).length} default
                </p>
              </CardContent>
            </Card>

            <Card className={CARD_CLASS}>
              <CardHeader className={CARD_HEADER_CLASS}>
                <CardTitle className={CARD_TITLE_CLASS}>Samples in range</CardTitle>
                <Activity className="h-4 w-4 text-gray-400" aria-hidden="true" />
              </CardHeader>
              <CardContent className="px-5 py-4">
                <div className="text-2xl font-semibold text-gray-900">{samples.length}</div>
                <p className="mt-1 text-xs text-gray-500">
                  {metric} on {nodeName}
                </p>
              </CardContent>
            </Card>
          </div>

          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Latest agent sample</CardTitle>
              <span className="text-xs text-gray-500" suppressHydrationWarning>
                {formatTimestamp(nodeMetrics?.timestamp)}
              </span>
            </CardHeader>
            <CardContent className="px-5 py-4">
              {nodeMetrics ? (
                <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
                  <div>
                    <dt className="text-xs text-gray-500">CPU</dt>
                    <dd className="text-sm font-medium text-gray-900">
                      {finiteOrNull(nodeMetrics.cpu_percent) === null
                        ? UNAVAILABLE
                        : `${formatNumber(nodeMetrics.cpu_percent, 1)}%`}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs text-gray-500">Memory</dt>
                    <dd className="text-sm font-medium text-gray-900">
                      {formatUsage(nodeMetrics.ram_used, nodeMetrics.ram_total) || UNAVAILABLE}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs text-gray-500">Disk</dt>
                    <dd className="text-sm font-medium text-gray-900">
                      {formatUsage(nodeMetrics.disk_used, nodeMetrics.disk_total) || UNAVAILABLE}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs text-gray-500">Load (1m)</dt>
                    <dd className="text-sm font-medium text-gray-900">
                      {formatNumber(nodeMetrics.load1)}
                    </dd>
                  </div>
                </dl>
              ) : (
                <div className="py-6 text-center">
                  <p className="text-sm font-semibold text-gray-900">No agent sample for this node</p>
                  <p className="mx-auto mt-1 max-w-xl text-sm text-gray-600">
                    {nodeMetricsError ||
                      'GET /api/v1/servers/{id}/metrics answered with nothing for this node.'}
                  </p>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="metrics" className="space-y-4">
          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Recorded samples</CardTitle>
              <div className="flex flex-wrap items-center gap-2">
                <label htmlFor="monitoring-metric" className="sr-only">
                  Metric
                </label>
                <select
                  id="monitoring-metric"
                  value={metric}
                  onChange={(event) => setMetric(event.target.value)}
                  className={SELECT_CLASS}
                >
                  {metricOptions.map((option) => (
                    <option key={option} value={option}>
                      {option}
                    </option>
                  ))}
                </select>
                <label htmlFor="monitoring-range" className="sr-only">
                  Time range
                </label>
                <select
                  id="monitoring-range"
                  value={range}
                  onChange={(event) => setRange(event.target.value)}
                  className={SELECT_CLASS}
                >
                  {RANGES.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </div>
            </CardHeader>
            <CardContent className="p-0">
              {samplesError ? (
                <p className="border-b border-gray-200 bg-red-50 px-5 py-3 text-sm text-red-700" role="alert">
                  {samplesError}
                </p>
              ) : null}
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50">
                    <tr className="[&_th]:border-b [&_th]:border-gray-200">
                      <th className={TH_CLASS}>Metric</th>
                      <th className={TH_CLASS}>Value</th>
                      <th className={TH_CLASS}>Recorded</th>
                    </tr>
                  </thead>
                  <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                    {samplesLoading ? (
                      <tr>
                        <td colSpan={3} className="px-4 py-12 text-center text-sm text-gray-600">
                          Loading samples...
                        </td>
                      </tr>
                    ) : samples.length === 0 ? (
                      <tr>
                        <td colSpan={3} className="px-4 py-12 text-center">
                          <p className="text-sm font-semibold text-gray-900">
                            No samples named “{metric}” for {nodeName} in this range
                          </p>
                          <p className="mx-auto mt-1 max-w-2xl text-sm text-gray-600">
                            This table is filled by POST
                            /api/v1/monitoring/servers/{'{'}id{'}'}/metrics. Nothing in the panel or
                            the node agent posts to it yet, so it stays empty until something does —
                            the live figures on the Overview tab come from the agent’s own sample
                            instead.
                          </p>
                        </td>
                      </tr>
                    ) : (
                      samples.map((sample) => (
                        <tr key={sample.id} className="hover:bg-gray-50">
                          <td className="px-4 py-3 text-sm text-gray-700">{sample.metric || UNAVAILABLE}</td>
                          <td className="px-4 py-3 text-sm font-medium text-gray-900">
                            {formatNumber(sample.value)} {sample.unit || ''}
                          </td>
                          <td className="px-4 py-3 text-sm text-gray-600" suppressHydrationWarning>
                            {formatTimestamp(sample.timestamp)}
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
          {alertActionError ? (
            <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
              {alertActionError}
            </div>
          ) : null}
          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Alert rules</CardTitle>
              <span className="text-xs text-gray-500">
                {alerts.length} rule{alerts.length === 1 ? '' : 's'}
              </span>
            </CardHeader>
            <CardContent className="px-5 py-4">
              {alertsError ? (
                <p className="mb-3 text-sm text-red-700" role="alert">
                  {alertsError}
                </p>
              ) : null}
              {alerts.length === 0 ? (
                <div className="py-8 text-center">
                  <Bell className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                  <p className="mt-3 text-sm font-semibold text-gray-900">No alert rules</p>
                  <p className="mt-1 text-sm text-gray-600">
                    A rule compares one metric on one node against a threshold.
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {alerts.map((alert) => (
                    <div
                      key={alert.id}
                      className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-gray-200 px-4 py-3"
                    >
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-gray-900">{alert.name || UNAVAILABLE}</p>
                        <p className="mt-0.5 text-xs text-gray-500">
                          {alert.metric || UNAVAILABLE} {alert.condition || ''}{' '}
                          {finiteOrNull(alert.threshold) === null ? '' : alert.threshold}
                        </p>
                        {alert.server_id ? null : (
                          <p className="mt-1 inline-flex items-center gap-1 text-xs text-amber-700">
                            <AlertTriangle size={12} aria-hidden="true" />
                            No node on this rule, so it is never evaluated.
                          </p>
                        )}
                        {alert.last_triggered_at ? (
                          <p className="mt-0.5 text-xs text-gray-500" suppressHydrationWarning>
                            Last fired {formatTimestamp(alert.last_triggered_at)}
                          </p>
                        ) : null}
                      </div>
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant="outline" className={`${BADGE_BASE} ${severityBadgeClass(alert.severity)}`}>
                          {alert.severity || 'unspecified'}
                        </Badge>
                        <Badge variant="outline" className={`${BADGE_BASE} ${statusBadgeClass(alert.status)}`}>
                          {alert.status || 'unknown'}
                        </Badge>
                        <Button
                          type="button"
                          variant="secondary"
                          size="sm"
                          onClick={() => void toggleAlert(alert)}
                          disabled={alertBusyId === alert.id}
                        >
                          {alert.is_active ? 'Disable' : 'Enable'}
                        </Button>
                        <Button
                          type="button"
                          variant="danger-outline"
                          size="sm"
                          onClick={() => void deleteAlert(alert)}
                          disabled={alertBusyId === alert.id}
                        >
                          <Trash2 size={14} aria-hidden="true" />
                          Delete
                        </Button>
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
              <span className="text-xs text-gray-500">
                {dashboards.length} dashboard{dashboards.length === 1 ? '' : 's'}
              </span>
            </CardHeader>
            <CardContent className="px-5 py-4">
              {dashboardsError ? (
                <p className="mb-3 text-sm text-red-700" role="alert">
                  {dashboardsError}
                </p>
              ) : null}
              {dashboards.length === 0 ? (
                <div className="py-8 text-center">
                  <Activity className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                  <p className="mt-3 text-sm font-semibold text-gray-900">No dashboards</p>
                  <p className="mx-auto mt-1 max-w-xl text-sm text-gray-600">
                    The API stores a dashboard’s layout and widgets as free-form JSON and the panel
                    has no editor for them yet, so dashboards created elsewhere are listed here but
                    cannot be built here.
                  </p>
                </div>
              ) : (
                <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
                  {dashboards.map((dashboard) => (
                    <div key={dashboard.id} className="rounded-md border border-gray-200 p-4">
                      <p className="text-sm font-semibold text-gray-900">{dashboard.name || UNAVAILABLE}</p>
                      <p className="mt-1 text-sm text-gray-600">
                        {dashboard.description || 'No description'}
                      </p>
                      {dashboard.is_default ? (
                        <Badge variant="outline" className={`${BADGE_BASE} mt-3 bg-brand-50 text-brand-700`}>
                          Default
                        </Badge>
                      ) : null}
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <AlertFormDialog
        open={alertDialogOpen}
        servers={servers}
        defaultServerId={serverId}
        metricSuggestions={metricOptions}
        onClose={() => setAlertDialogOpen(false)}
        onCreated={() => void loadAlerts()}
      />
    </div>
  );
}
