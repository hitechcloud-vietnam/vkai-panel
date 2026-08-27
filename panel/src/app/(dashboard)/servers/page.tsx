'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import {
  Server,
  Plus,
  Search,
  Filter,
  Cpu,
  MemoryStick,
  HardDrive,
  RefreshCw,
} from 'lucide-react';
import { api } from '@/services/api';

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
  agent_version: string;
  last_heartbeat: string;
  metrics?: {
    cpu_percent: number;
    ram_used: number;
    ram_total: number;
    disk_used: number;
    disk_total: number;
    net_in: number;
    net_out: number;
  };
}

export default function ServersPage() {
  const [servers, setServers] = useState<ServerInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [search, setSearch] = useState('');
  const [heartbeats, setHeartbeats] = useState<Record<string, string>>({});

  useEffect(() => {
    loadServers();
  }, []);

  // Format timestamps on the client only, to avoid SSR/CSR hydration drift.
  useEffect(() => {
    const map: Record<string, string> = {};
    servers.forEach((s) => {
      if (!s?.id) return;
      if (s?.last_heartbeat) {
        const d = new Date(s.last_heartbeat);
        map[s.id] = Number.isNaN(d.getTime()) ? 'Never' : d.toLocaleString();
      } else {
        map[s.id] = 'Never';
      }
    });
    setHeartbeats(map);
  }, [servers]);

  const loadServers = async () => {
    setLoading(true);
    setError('');
    try {
      const response = await api.get('/api/v1/servers');
      setServers(Array.isArray(response?.data?.data) ? response.data.data : []);
    } catch (err: any) {
      console.error('Failed to load servers:', err);
      setServers([]);
      setError(err?.response?.data?.error || err?.message || 'Failed to load servers');
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

  const percent = (used?: number, total?: number) => {
    const u = Number(used);
    const t = Number(total);
    if (!Number.isFinite(u) || !Number.isFinite(t) || t <= 0) return 0;
    return Math.min(100, Math.max(0, (u / t) * 100));
  };

  const query = search.trim().toLowerCase();
  const filteredServers = servers.filter(
    (s) =>
      (s?.name || '').toLowerCase().includes(query) ||
      (s?.hostname || '').toLowerCase().includes(query) ||
      (s?.ip_address || '').includes(search.trim())
  );

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-b-brand-600" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Servers</h1>
          <p className="text-sm text-gray-600 mt-1">Manage your servers and infrastructure</p>
        </div>
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={loadServers}
            className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            <RefreshCw size={16} aria-hidden="true" />
            Refresh
          </button>
          <Link
            href="/servers/add"
            className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <Plus size={16} aria-hidden="true" />
            Add Server
          </Link>
        </div>
      </div>

      {error && (
        <div
          role="alert"
          className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
        >
          {error}
        </div>
      )}

      {/* Search & Filter */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm px-4 py-3">
        <div className="flex items-center gap-3">
          <div className="flex-1 relative">
            <Search
              className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
              size={16}
              aria-hidden="true"
            />
            <input
              id="server-search"
              type="text"
              aria-label="Search servers"
              placeholder="Search servers..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full rounded-md border border-gray-300 bg-white pl-9 pr-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus:outline-none"
            />
          </div>
          <button
            type="button"
            className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            <Filter size={16} aria-hidden="true" />
            Filter
          </button>
        </div>
      </div>

      {/* Server Grid */}
      {filteredServers.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded-lg shadow-sm text-center px-6 py-14">
          <Server className="mx-auto text-gray-300" size={40} aria-hidden="true" />
          <h3 className="mt-4 text-sm font-semibold text-gray-900">No servers found</h3>
          <p className="mt-1 text-sm text-gray-600">
            {search ? 'Try a different search term' : 'Add your first server to get started'}
          </p>
          {!search && (
            <Link
              href="/servers/add"
              className="mt-4 inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
            >
              <Plus size={16} aria-hidden="true" />
              Add Server
            </Link>
          )}
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          {filteredServers.map((server) => (
            <Link
              key={server?.id}
              href={`/servers/${server?.id}`}
              className="block bg-white border border-gray-200 rounded-lg shadow-sm p-5 hover:border-gray-300 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              <div className="flex items-start justify-between mb-4">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-gray-50 border border-gray-200 rounded-md">
                    <Server className="text-gray-600" size={18} aria-hidden="true" />
                  </div>
                  <div>
                    <h3 className="text-sm font-semibold text-gray-900">
                      {server?.name || '-'}
                    </h3>
                    <p className="text-xs text-gray-500">{server?.hostname || '-'}</p>
                  </div>
                </div>
                <span
                  className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${getStatusBadge(
                    server?.status || ''
                  )}`}
                >
                  {server?.status || 'unknown'}
                </span>
              </div>

              <div className="grid grid-cols-2 gap-4 mb-4">
                <div>
                  <p className="text-xs text-gray-500">IP Address</p>
                  <p className="text-sm font-mono text-gray-900">{server?.ip_address || '-'}</p>
                </div>
                <div>
                  <p className="text-xs text-gray-500">OS</p>
                  <p className="text-sm text-gray-900">{server?.os || '-'}</p>
                </div>
                <div>
                  <p className="text-xs text-gray-500">Agent Version</p>
                  <p className="text-sm text-gray-900">{server?.agent_version || 'N/A'}</p>
                </div>
                <div>
                  <p className="text-xs text-gray-500">Last Heartbeat</p>
                  <p className="text-sm text-gray-900" suppressHydrationWarning>
                    {heartbeats[server?.id] || 'Never'}
                  </p>
                </div>
              </div>

              {/* Resource Usage */}
              <div className="space-y-3">
                <div>
                  <div className="flex items-center justify-between mb-1">
                    <div className="flex items-center gap-2">
                      <Cpu size={12} className="text-gray-500" aria-hidden="true" />
                      <span className="text-xs text-gray-500">CPU</span>
                    </div>
                    <span className="text-xs text-gray-700">
                      {(server?.metrics?.cpu_percent ?? 0).toFixed(1)}%
                    </span>
                  </div>
                  <div className="h-1.5 w-full rounded-md bg-gray-100 overflow-hidden">
                    <div
                      className="h-full rounded-md bg-brand-600"
                      style={{ width: `${percent(server?.metrics?.cpu_percent ?? 0, 100)}%` }}
                    />
                  </div>
                </div>

                <div>
                  <div className="flex items-center justify-between mb-1">
                    <div className="flex items-center gap-2">
                      <MemoryStick size={12} className="text-gray-500" aria-hidden="true" />
                      <span className="text-xs text-gray-500">RAM</span>
                    </div>
                    <span className="text-xs text-gray-700">
                      {formatBytes(server?.metrics?.ram_used ?? 0)} /{' '}
                      {formatBytes(server?.ram_total ?? 0)}
                    </span>
                  </div>
                  <div className="h-1.5 w-full rounded-md bg-gray-100 overflow-hidden">
                    <div
                      className="h-full rounded-md bg-emerald-600"
                      style={{
                        width: `${percent(server?.metrics?.ram_used, server?.ram_total)}%`,
                      }}
                    />
                  </div>
                </div>

                <div>
                  <div className="flex items-center justify-between mb-1">
                    <div className="flex items-center gap-2">
                      <HardDrive size={12} className="text-gray-500" aria-hidden="true" />
                      <span className="text-xs text-gray-500">Disk</span>
                    </div>
                    <span className="text-xs text-gray-700">
                      {formatBytes(server?.metrics?.disk_used ?? 0)} /{' '}
                      {formatBytes(server?.disk_total ?? 0)}
                    </span>
                  </div>
                  <div className="h-1.5 w-full rounded-md bg-gray-100 overflow-hidden">
                    <div
                      className="h-full rounded-md bg-gray-600"
                      style={{
                        width: `${percent(server?.metrics?.disk_used, server?.disk_total)}%`,
                      }}
                    />
                  </div>
                </div>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
