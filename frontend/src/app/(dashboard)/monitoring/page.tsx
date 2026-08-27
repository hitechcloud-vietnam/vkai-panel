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

export default function MonitoringPage() {
  const [metrics, setMetrics] = useState<Metric[]>([]);
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [dashboards, setDashboards] = useState<Dashboard[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState('overview');

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      setLoading(true);
      const [metricsRes, alertsRes, dashboardsRes] = await Promise.all([
        monitoringApi.getMetrics({ limit: 100 }),
        monitoringApi.getAlerts(),
        monitoringApi.getDashboards(),
      ]);
      setMetrics(metricsRes.data.metrics || []);
      setAlerts(alertsRes.data.alerts || []);
      setDashboards(dashboardsRes.data.dashboards || []);
    } catch (error) {
      console.error('Failed to fetch monitoring data:', error);
    } finally {
      setLoading(false);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'ok':
      case 'active':
        return 'bg-green-100 text-green-800';
      case 'warning':
        return 'bg-yellow-100 text-yellow-800';
      case 'critical':
      case 'firing':
        return 'bg-red-100 text-red-800';
      default:
        return 'bg-gray-100 text-gray-800';
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
    return labels[type] || type;
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <RefreshCw className="h-8 w-8 animate-spin text-gray-400" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">Monitoring</h1>
          <p className="text-gray-500">System metrics, alerts, and dashboards</p>
        </div>
        <div className="flex space-x-2">
          <Button variant="outline" onClick={fetchData}>
            <RefreshCw className="h-4 w-4 mr-2" />
            Refresh
          </Button>
          <Button>
            <Plus className="h-4 w-4 mr-2" />
            Add Alert
          </Button>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="metrics">Metrics</TabsTrigger>
          <TabsTrigger value="alerts">Alerts</TabsTrigger>
          <TabsTrigger value="dashboards">Dashboards</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Active Alerts</CardTitle>
                <Bell className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">
                  {alerts.filter((a) => a.status === 'firing').length}
                </div>
                <p className="text-xs text-muted-foreground">
                  {alerts.filter((a) => a.status === 'ok').length} resolved
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Dashboards</CardTitle>
                <Activity className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">{dashboards.length}</div>
                <p className="text-xs text-muted-foreground">
                  {dashboards.filter((d) => d.is_default).length} default
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Metrics Collected</CardTitle>
                <Settings className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">{metrics.length}</div>
                <p className="text-xs text-muted-foreground">Last 24 hours</p>
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Recent Metrics</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {metrics.slice(0, 10).map((metric) => (
                  <div
                    key={metric.id}
                    className="flex items-center justify-between p-3 bg-gray-50 rounded-lg"
                  >
                    <div>
                      <p className="font-medium">{getMetricTypeLabel(metric.metric_type)}</p>
                      <p className="text-sm text-gray-500">
                        Server: {metric.server_id.slice(0, 8)}...
                      </p>
                    </div>
                    <div className="text-right">
                      <p className="font-bold">
                        {metric.value.toFixed(2)} {metric.unit}
                      </p>
                      <p className="text-xs text-gray-500">
                        {new Date(metric.recorded_at).toLocaleString()}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="metrics" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>All Metrics</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b">
                      <th className="text-left p-2">Type</th>
                      <th className="text-left p-2">Server</th>
                      <th className="text-left p-2">Value</th>
                      <th className="text-left p-2">Time</th>
                    </tr>
                  </thead>
                  <tbody>
                    {metrics.map((metric) => (
                      <tr key={metric.id} className="border-b">
                        <td className="p-2">{getMetricTypeLabel(metric.metric_type)}</td>
                        <td className="p-2 font-mono text-sm">
                          {metric.server_id.slice(0, 8)}...
                        </td>
                        <td className="p-2 font-medium">
                          {metric.value.toFixed(2)} {metric.unit}
                        </td>
                        <td className="p-2 text-sm text-gray-500">
                          {new Date(metric.recorded_at).toLocaleString()}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="alerts" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Alert Rules</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {alerts.map((alert) => (
                  <div
                    key={alert.id}
                    className="flex items-center justify-between p-4 border rounded-lg"
                  >
                    <div>
                      <p className="font-medium">{alert.name}</p>
                      <p className="text-sm text-gray-500">
                        {getMetricTypeLabel(alert.metric_type)} {alert.condition}{' '}
                        {alert.threshold}
                      </p>
                    </div>
                    <div className="flex items-center space-x-2">
                      <Badge className={getStatusColor(alert.status)}>{alert.status}</Badge>
                      <Badge variant={alert.enabled ? 'default' : 'secondary'}>
                        {alert.enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="dashboards" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Dashboards</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {dashboards.map((dashboard) => (
                  <Card key={dashboard.id}>
                    <CardHeader>
                      <CardTitle className="text-lg">{dashboard.name}</CardTitle>
                    </CardHeader>
                    <CardContent>
                      <p className="text-sm text-gray-500">{dashboard.description}</p>
                      {dashboard.is_default && (
                        <Badge className="mt-2">Default</Badge>
                      )}
                    </CardContent>
                  </Card>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
