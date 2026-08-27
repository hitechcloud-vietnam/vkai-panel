'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import {
  Server,
  Globe,
  Database,
  Activity,
  Cpu,
  MemoryStick,
  HardDrive,
  ShieldCheck,
  AlertTriangle,
  CheckCircle,
  Plus,
} from 'lucide-react';
import { api } from '@/services/api';
import { useAuthStore } from '@/store/auth';
import { brand } from '@/lib/brand';

interface DashboardStats {
  servers: number;
  websites: number;
  databases: number;
  sslCerts: number;
}

interface ServerInfo {
  id: string;
  name: string;
  hostname: string;
  status: string;
  ip_address: string;
  os: string;
  cpu_cores: number;
  ram_total: number;
  disk_total: number;
  metrics?: {
    cpu_percent: number;
    ram_used: number;
    disk_used: number;
  };
}

export default function DashboardPage() {
  const { user } = useAuthStore();
  const [stats, setStats] = useState<DashboardStats>({
    servers: 0,
    websites: 0,
    databases: 0,
    sslCerts: 0,
  });
  const [servers, setServers] = useState<ServerInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    loadDashboardData();
  }, []);

  const loadDashboardData = async () => {
    setLoading(true);
    setError('');
    try {
      const [serversRes] = await Promise.all([
        api.get('/api/v1/servers'),
      ]);

      const list = Array.isArray(serversRes?.data?.data) ? serversRes.data.data : [];
      setServers(list);
      setStats({
        servers: list.length,
        websites: 0,
        databases: 0,
        sslCerts: 0,
      });
    } catch (err: any) {
      console.error('Failed to load dashboard data:', err);
      setServers([]);
      setError(
        err?.response?.data?.error || err?.message || 'Failed to load dashboard data'
      );
    } finally {
      setLoading(false);
    }
  };

  const formatBytes = (bytes: number) => {
    const value = Number(bytes);
    if (!Number.isFinite(value) || value <= 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(Math.floor(Math.log(value) / Math.log(k)), sizes.length - 1);
    return parseFloat((value / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'online':
        return 'text-emerald-600';
      case 'offline':
        return 'text-red-600';
      default:
        return 'text-amber-600';
    }
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'online':
        return 'bg-emerald-50 text-emerald-700';
      case 'offline':
        return 'bg-red-50 text-red-700';
      default:
        return 'bg-amber-50 text-amber-700';
    }
  };

  const onlineCount = servers.filter((s) => s?.status === 'online').length;
  const offlineCount = servers.filter((s) => s?.status === 'offline').length;

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-b-blue-600" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">
            Welcome back, {user?.first_name || user?.username || 'there'}
          </h1>
          <p className="text-sm text-gray-600 mt-1">
            Here&apos;s what&apos;s happening with your servers today.
          </p>
        </div>
        <Link
          href="/servers/add"
          className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2"
        >
          <Plus size={16} aria-hidden="true" />
          Add Server
        </Link>
      </div>

      {error && (
        <div
          role="alert"
          className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
        >
          {error}
        </div>
      )}

      {/* Stats Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-5">
          <div className="flex items-start justify-between">
            <div>
              <p className="text-sm text-gray-500">Servers</p>
              <p className="mt-1 text-2xl font-semibold text-gray-900">{stats.servers}</p>
            </div>
            <div className="p-2 bg-gray-50 border border-gray-200 rounded-md">
              <Server className="text-gray-600" size={20} aria-hidden="true" />
            </div>
          </div>
          <div className="mt-4 flex items-center gap-2 text-sm">
            <CheckCircle className="text-emerald-600" size={14} aria-hidden="true" />
            <span className="text-gray-600">{onlineCount} online</span>
            {offlineCount > 0 && (
              <>
                <span className="text-gray-300">|</span>
                <AlertTriangle className="text-red-600" size={14} aria-hidden="true" />
                <span className="text-red-700">{offlineCount} offline</span>
              </>
            )}
          </div>
        </div>

        <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-5">
          <div className="flex items-start justify-between">
            <div>
              <p className="text-sm text-gray-500">Websites</p>
              <p className="mt-1 text-2xl font-semibold text-gray-900">{stats.websites}</p>
            </div>
            <div className="p-2 bg-gray-50 border border-gray-200 rounded-md">
              <Globe className="text-gray-600" size={20} aria-hidden="true" />
            </div>
          </div>
          <div className="mt-4 flex items-center gap-2 text-sm">
            <CheckCircle className="text-emerald-600" size={14} aria-hidden="true" />
            <span className="text-gray-600">All active</span>
          </div>
        </div>

        <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-5">
          <div className="flex items-start justify-between">
            <div>
              <p className="text-sm text-gray-500">Databases</p>
              <p className="mt-1 text-2xl font-semibold text-gray-900">{stats.databases}</p>
            </div>
            <div className="p-2 bg-gray-50 border border-gray-200 rounded-md">
              <Database className="text-gray-600" size={20} aria-hidden="true" />
            </div>
          </div>
          <div className="mt-4 flex items-center gap-2 text-sm">
            <Activity className="text-gray-500" size={14} aria-hidden="true" />
            <span className="text-gray-600">MySQL, PostgreSQL, Redis</span>
          </div>
        </div>

        <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-5">
          <div className="flex items-start justify-between">
            <div>
              <p className="text-sm text-gray-500">SSL Certificates</p>
              <p className="mt-1 text-2xl font-semibold text-gray-900">{stats.sslCerts}</p>
            </div>
            <div className="p-2 bg-gray-50 border border-gray-200 rounded-md">
              <ShieldCheck className="text-gray-600" size={20} aria-hidden="true" />
            </div>
          </div>
          <div className="mt-4 flex items-center gap-2 text-sm">
            <CheckCircle className="text-emerald-600" size={14} aria-hidden="true" />
            <span className="text-gray-600">All valid</span>
          </div>
        </div>
      </div>

      {/* Server List */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-200">
          <h2 className="text-sm font-semibold text-gray-900">Servers</h2>
          <Link
            href="/servers/add"
            className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
          >
            <Plus size={14} aria-hidden="true" />
            Add Server
          </Link>
        </div>

        {servers.length === 0 ? (
          <div className="text-center px-6 py-12">
            <Server className="mx-auto text-gray-300" size={40} aria-hidden="true" />
            <h3 className="mt-4 text-sm font-semibold text-gray-900">No servers yet</h3>
            <p className="mt-1 text-sm text-gray-600">
              Add your first server to get started with {brand.productName}.
            </p>
            <Link
              href="/servers/add"
              className="mt-4 inline-flex items-center gap-2 rounded-md bg-blue-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2"
            >
              <Plus size={16} aria-hidden="true" />
              Add Your First Server
            </Link>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[900px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Server</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Status</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">IP Address</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">OS</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">CPU</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">RAM</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Disk</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Actions</th>
                </tr>
              </thead>
              <tbody>
                {servers.map((server) => (
                  <tr
                    key={server?.id}
                    className="border-b border-gray-100 hover:bg-gray-50"
                  >
                    <td className="px-4 py-3 text-sm text-gray-700">
                      <div>
                        <p className="font-medium text-gray-900">{server?.name || '-'}</p>
                        <p className="text-xs text-gray-500">{server?.hostname || '-'}</p>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-sm">
                      <span
                        className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${getStatusBadge(
                          server?.status || ''
                        )}`}
                      >
                        {server?.status || 'unknown'}
                      </span>
                    </td>
                    <td className="px-4 py-3 font-mono text-sm text-gray-700">
                      {server?.ip_address || '-'}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700">{server?.os || '-'}</td>
                    <td className="px-4 py-3 text-sm text-gray-700">
                      <div className="flex items-center gap-2">
                        <Cpu size={14} className="text-gray-500" aria-hidden="true" />
                        <span>{(server?.metrics?.cpu_percent ?? 0).toFixed(1)}%</span>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700">
                      <div className="flex items-center gap-2">
                        <MemoryStick size={14} className="text-gray-500" aria-hidden="true" />
                        <span>
                          {formatBytes(server?.metrics?.ram_used ?? 0)} /{' '}
                          {formatBytes(server?.ram_total ?? 0)}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700">
                      <div className="flex items-center gap-2">
                        <HardDrive size={14} className="text-gray-500" aria-hidden="true" />
                        <span>
                          {formatBytes(server?.metrics?.disk_used ?? 0)} /{' '}
                          {formatBytes(server?.disk_total ?? 0)}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-sm">
                      <Link
                        href={`/servers/${server?.id}`}
                        className="inline-flex items-center rounded-md border border-gray-300 bg-white px-2.5 py-1 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                      >
                        View
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Quick Actions */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Link
          href="/websites"
          className="bg-white border border-gray-200 rounded-lg shadow-sm p-5 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
        >
          <div className="flex items-center gap-4">
            <div className="p-2 bg-gray-50 border border-gray-200 rounded-md">
              <Globe className="text-gray-600" size={20} aria-hidden="true" />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-gray-900">Add Website</h3>
              <p className="text-sm text-gray-600">Deploy a new website or app</p>
            </div>
          </div>
        </Link>

        <Link
          href="/databases"
          className="bg-white border border-gray-200 rounded-lg shadow-sm p-5 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
        >
          <div className="flex items-center gap-4">
            <div className="p-2 bg-gray-50 border border-gray-200 rounded-md">
              <Database className="text-gray-600" size={20} aria-hidden="true" />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-gray-900">Create Database</h3>
              <p className="text-sm text-gray-600">MySQL, PostgreSQL, or Redis</p>
            </div>
          </div>
        </Link>

        <Link
          href="/logs"
          className="bg-white border border-gray-200 rounded-lg shadow-sm p-5 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
        >
          <div className="flex items-center gap-4">
            <div className="p-2 bg-gray-50 border border-gray-200 rounded-md">
              <Activity className="text-gray-600" size={20} aria-hidden="true" />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-gray-900">View Logs</h3>
              <p className="text-sm text-gray-600">Check server and app logs</p>
            </div>
          </div>
        </Link>
      </div>
    </div>
  );
}
