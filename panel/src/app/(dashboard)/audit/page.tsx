'use client';

import { useState, useEffect } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Shield, Search, RefreshCw, Download, Trash2 } from 'lucide-react';
import { auditApi } from '@/services/api';

interface AuditLog {
  id: string;
  user_id: string;
  tenant_id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  details: Record<string, any>;
  ip_address: string;
  user_agent: string;
  created_at: string;
}

interface AuditStats {
  total_logs: number;
  actions: Record<string, number>;
  resources: Record<string, number>;
  top_users: Array<{ user_id: string; count: number }>;
}

const CARD_CLASS = 'rounded-lg border border-gray-200 bg-white shadow-sm';
const CARD_HEADER_CLASS = 'flex flex-row items-center justify-between space-y-0 border-b border-gray-200 px-5 py-4';
const CARD_TITLE_CLASS = 'text-sm font-semibold text-gray-900';
const BTN_PRIMARY =
  'bg-blue-600 text-white hover:bg-blue-700 focus-visible:ring-blue-500';
const BTN_SECONDARY =
  'border-gray-300 bg-white text-gray-700 hover:bg-gray-50 focus-visible:ring-blue-500';
const BTN_DANGER = 'bg-red-600 text-white hover:bg-red-700 focus-visible:ring-red-500';
const INPUT_CLASS =
  'w-full rounded-md border-gray-300 bg-white text-sm text-gray-900 placeholder:text-gray-400 focus-visible:ring-1 focus-visible:ring-blue-500 focus-visible:ring-offset-0';
const SELECT_CLASS =
  'border-gray-300 bg-white text-sm text-gray-900 focus:ring-1 focus:ring-blue-500 focus:ring-offset-0';
const BADGE_BASE = 'rounded-md border-transparent px-2 py-0.5 text-xs font-medium';

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

export default function AuditPage() {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [stats, setStats] = useState<AuditStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [actionFilter, setActionFilter] = useState('all');
  const [resourceFilter, setResourceFilter] = useState('all');

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      setLoading(true);
      setError(null);
      const [logsRes, statsRes] = await Promise.all([
        auditApi.search({ limit: 100 }),
        auditApi.getStats(),
      ]);
      setLogs(Array.isArray(logsRes?.data?.logs) ? logsRes.data.logs : []);
      setStats(statsRes?.data ?? null);
    } catch (err: any) {
      console.error('Failed to fetch audit logs:', err);
      setLogs([]);
      setStats(null);
      setError(err?.response?.data?.message || 'Failed to load audit logs');
    } finally {
      setLoading(false);
    }
  };

  const handleSearch = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await auditApi.search({
        query: searchQuery,
        action: actionFilter === 'all' ? undefined : actionFilter,
        resource_type: resourceFilter === 'all' ? undefined : resourceFilter,
        limit: 100,
      });
      setLogs(Array.isArray(res?.data?.logs) ? res.data.logs : []);
    } catch (err: any) {
      console.error('Failed to search audit logs:', err);
      setLogs([]);
      setError(err?.response?.data?.message || 'Failed to search audit logs');
    } finally {
      setLoading(false);
    }
  };

  const cleanupOldLogs = async () => {
    if (!confirm('Are you sure you want to delete audit logs older than 90 days?')) return;
    try {
      setError(null);
      await auditApi.cleanup({ days: 90 });
      fetchData();
    } catch (err: any) {
      console.error('Failed to cleanup audit logs:', err);
      setError(err?.response?.data?.message || 'Failed to cleanup audit logs');
    }
  };

  const getActionColor = (action: string) => {
    const value = (action || '').toLowerCase();
    if (value.includes('create') || value.includes('add')) return 'bg-emerald-50 text-emerald-700';
    if (value.includes('update') || value.includes('edit')) return 'bg-blue-50 text-blue-700';
    if (value.includes('delete') || value.includes('remove')) return 'bg-red-50 text-red-700';
    if (value.includes('login') || value.includes('auth')) return 'bg-sky-50 text-sky-700';
    return 'bg-gray-100 text-gray-700';
  };

  const actionEntries = Object.entries(stats?.actions ?? {});
  const resourceEntries = Object.entries(stats?.resources ?? {});
  const topUsers = Array.isArray(stats?.top_users) ? stats!.top_users : [];

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <RefreshCw className="h-6 w-6 animate-spin text-gray-400" aria-hidden="true" />
        <span className="ml-2 text-sm text-gray-600">Loading audit logs...</span>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Audit Logs</h1>
          <p className="mt-1 text-sm text-gray-600">Track all system activities and changes</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="outline" onClick={fetchData} className={BTN_SECONDARY}>
            <RefreshCw className="mr-2 h-4 w-4" aria-hidden="true" />
            Refresh
          </Button>
          <Button variant="outline" className={BTN_SECONDARY}>
            <Download className="mr-2 h-4 w-4" aria-hidden="true" />
            Export
          </Button>
          <Button variant="destructive" onClick={cleanupOldLogs} className={BTN_DANGER}>
            <Trash2 className="mr-2 h-4 w-4" aria-hidden="true" />
            Cleanup
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700" role="alert">
          {error}
        </div>
      )}

      {stats && (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Total Logs</CardTitle>
              <Shield className="h-4 w-4 text-gray-400" aria-hidden="true" />
            </CardHeader>
            <CardContent className="px-5 py-4">
              <div className="text-2xl font-semibold text-gray-900">{stats.total_logs ?? 0}</div>
            </CardContent>
          </Card>

          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Top Action</CardTitle>
            </CardHeader>
            <CardContent className="px-5 py-4">
              <div className="truncate text-2xl font-semibold text-gray-900">
                {actionEntries.sort(([, a], [, b]) => b - a)[0]?.[0] || 'N/A'}
              </div>
            </CardContent>
          </Card>

          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Top Resource</CardTitle>
            </CardHeader>
            <CardContent className="px-5 py-4">
              <div className="truncate text-2xl font-semibold text-gray-900">
                {resourceEntries.sort(([, a], [, b]) => b - a)[0]?.[0] || 'N/A'}
              </div>
            </CardContent>
          </Card>

          <Card className={CARD_CLASS}>
            <CardHeader className={CARD_HEADER_CLASS}>
              <CardTitle className={CARD_TITLE_CLASS}>Active Users</CardTitle>
            </CardHeader>
            <CardContent className="px-5 py-4">
              <div className="text-2xl font-semibold text-gray-900">{topUsers.length}</div>
            </CardContent>
          </Card>
        </div>
      )}

      <Card className={CARD_CLASS}>
        <CardHeader className={CARD_HEADER_CLASS}>
          <CardTitle className={CARD_TITLE_CLASS}>Search Filters</CardTitle>
        </CardHeader>
        <CardContent className="px-5 py-4">
          <div className="flex flex-col gap-3 md:flex-row">
            <div className="flex-1">
              <label htmlFor="audit-search" className="sr-only">
                Search audit logs
              </label>
              <Input
                id="audit-search"
                placeholder="Search logs..."
                aria-label="Search audit logs"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                onKeyPress={(e) => e.key === 'Enter' && handleSearch()}
                className={INPUT_CLASS}
              />
            </div>
            <Select value={actionFilter} onValueChange={setActionFilter}>
              <SelectTrigger className={`w-full md:w-[180px] ${SELECT_CLASS}`} aria-label="Filter by action">
                <SelectValue placeholder="Action" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Actions</SelectItem>
                <SelectItem value="create">Create</SelectItem>
                <SelectItem value="update">Update</SelectItem>
                <SelectItem value="delete">Delete</SelectItem>
                <SelectItem value="login">Login</SelectItem>
              </SelectContent>
            </Select>
            <Select value={resourceFilter} onValueChange={setResourceFilter}>
              <SelectTrigger className={`w-full md:w-[180px] ${SELECT_CLASS}`} aria-label="Filter by resource">
                <SelectValue placeholder="Resource" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Resources</SelectItem>
                <SelectItem value="server">Server</SelectItem>
                <SelectItem value="website">Website</SelectItem>
                <SelectItem value="database">Database</SelectItem>
                <SelectItem value="user">User</SelectItem>
              </SelectContent>
            </Select>
            <Button onClick={handleSearch} className={BTN_PRIMARY}>
              <Search className="mr-2 h-4 w-4" aria-hidden="true" />
              Search
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card className={CARD_CLASS}>
        <CardHeader className={CARD_HEADER_CLASS}>
          <CardTitle className={CARD_TITLE_CLASS}>Audit Logs</CardTitle>
          <span className="text-xs text-gray-500">{logs.length} entries</span>
        </CardHeader>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50">
                <tr className="[&_th]:border-b [&_th]:border-gray-200">
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Timestamp</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Action</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Resource</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">User</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">IP Address</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Details</th>
                </tr>
              </thead>
              <tbody className="[&_td]:border-b [&_td]:border-gray-100">
                {logs.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="px-4 py-12 text-center">
                      <Shield className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
                      <p className="mt-3 text-sm font-semibold text-gray-900">No audit logs</p>
                      <p className="mt-1 text-sm text-gray-600">
                        Activity will appear here once actions are recorded.
                      </p>
                    </td>
                  </tr>
                ) : (
                  logs.map((log) => (
                    <tr key={log.id} className="hover:bg-gray-50">
                      <td className="px-4 py-3 text-sm text-gray-700" suppressHydrationWarning>
                        {formatTimestamp(log.created_at)}
                      </td>
                      <td className="px-4 py-3">
                        <Badge variant="outline" className={`${BADGE_BASE} ${getActionColor(log.action)}`}>
                          {log.action || '—'}
                        </Badge>
                      </td>
                      <td className="px-4 py-3 text-sm text-gray-700">
                        <span className="font-medium text-gray-900">{log.resource_type || '—'}</span>
                        {log.resource_id && (
                          <span className="ml-1 text-xs text-gray-500">({shortId(log.resource_id)})</span>
                        )}
                      </td>
                      <td className="px-4 py-3 font-mono text-sm text-gray-700">{shortId(log.user_id)}</td>
                      <td className="px-4 py-3 font-mono text-sm text-gray-700">{log.ip_address || '—'}</td>
                      <td className="px-4 py-3">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-blue-700 hover:bg-blue-50 hover:text-blue-700"
                        >
                          View
                        </Button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
