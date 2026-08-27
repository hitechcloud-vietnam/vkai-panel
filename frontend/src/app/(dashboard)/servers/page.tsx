'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import {
  Server,
  Plus,
  Search,
  Filter,
  MoreVertical,
  Cpu,
  MemoryStick,
  HardDrive,
  Network,
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
  const [search, setSearch] = useState('');

  useEffect(() => {
    loadServers();
  }, []);

  const loadServers = async () => {
    try {
      const response = await api.get('/api/v1/servers');
      setServers(response.data.data || []);
    } catch (error) {
      console.error('Failed to load servers:', error);
    } finally {
      setLoading(false);
    }
  };

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'online':
        return 'badge-success';
      case 'offline':
        return 'badge-error';
      default:
        return 'badge-warning';
    }
  };

  const filteredServers = servers.filter(
    (s) =>
      s.name.toLowerCase().includes(search.toLowerCase()) ||
      s.hostname.toLowerCase().includes(search.toLowerCase()) ||
      s.ip_address.includes(search)
  );

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Servers</h1>
          <p className="text-dark-400 mt-1">Manage your servers and infrastructure</p>
        </div>
        <div className="flex items-center gap-3">
          <button onClick={loadServers} className="btn btn-secondary">
            <RefreshCw size={16} />
            Refresh
          </button>
          <Link href="/servers/add" className="btn btn-primary">
            <Plus size={16} />
            Add Server
          </Link>
        </div>
      </div>

      {/* Search & Filter */}
      <div className="flex items-center gap-4">
        <div className="flex-1 relative">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-dark-400" size={16} />
          <input
            type="text"
            placeholder="Search servers..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-10 pr-4 py-2 bg-dark-800 border border-dark-600 rounded-lg text-sm text-dark-100 placeholder-dark-400 focus:outline-none focus:border-primary-500"
          />
        </div>
        <button className="btn btn-secondary">
          <Filter size={16} />
          Filter
        </button>
      </div>

      {/* Server Grid */}
      {filteredServers.length === 0 ? (
        <div className="text-center py-16">
          <Server className="mx-auto text-dark-600" size={64} />
          <h3 className="mt-4 text-xl font-medium text-dark-300">No servers found</h3>
          <p className="mt-2 text-dark-500">
            {search ? 'Try a different search term' : 'Add your first server to get started'}
          </p>
          {!search && (
            <Link href="/servers/add" className="mt-4 inline-flex btn btn-primary">
              <Plus size={16} />
              Add Server
            </Link>
          )}
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          {filteredServers.map((server) => (
            <Link
              key={server.id}
              href={`/servers/${server.id}`}
              className="card hover:border-primary-500/50 transition-colors"
            >
              <div className="flex items-start justify-between mb-4">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-dark-800 rounded-lg">
                    <Server className="text-primary-400" size={20} />
                  </div>
                  <div>
                    <h3 className="font-semibold text-dark-100">{server.name}</h3>
                    <p className="text-sm text-dark-500">{server.hostname}</p>
                  </div>
                </div>
                <span className={`badge ${getStatusBadge(server.status)}`}>
                  {server.status}
                </span>
              </div>

              <div className="grid grid-cols-2 gap-4 mb-4">
                <div>
                  <p className="text-xs text-dark-500">IP Address</p>
                  <p className="text-sm font-mono text-dark-200">{server.ip_address}</p>
                </div>
                <div>
                  <p className="text-xs text-dark-500">OS</p>
                  <p className="text-sm text-dark-200">{server.os}</p>
                </div>
                <div>
                  <p className="text-xs text-dark-500">Agent Version</p>
                  <p className="text-sm text-dark-200">{server.agent_version || 'N/A'}</p>
                </div>
                <div>
                  <p className="text-xs text-dark-500">Last Heartbeat</p>
                  <p className="text-sm text-dark-200">
                    {server.last_heartbeat
                      ? new Date(server.last_heartbeat).toLocaleString()
                      : 'Never'}
                  </p>
                </div>
              </div>

              {/* Resource Usage */}
              <div className="space-y-3">
                <div>
                  <div className="flex items-center justify-between mb-1">
                    <div className="flex items-center gap-2">
                      <Cpu size={12} className="text-dark-400" />
                      <span className="text-xs text-dark-400">CPU</span>
                    </div>
                    <span className="text-xs text-dark-300">
                      {server.metrics?.cpu_percent?.toFixed(1) || 0}%
                    </span>
                  </div>
                  <div className="progress-bar">
                    <div
                      className="progress-bar-fill bg-primary-500"
                      style={{ width: `${server.metrics?.cpu_percent || 0}%` }}
                    />
                  </div>
                </div>

                <div>
                  <div className="flex items-center justify-between mb-1">
                    <div className="flex items-center gap-2">
                      <MemoryStick size={12} className="text-dark-400" />
                      <span className="text-xs text-dark-400">RAM</span>
                    </div>
                    <span className="text-xs text-dark-300">
                      {server.metrics
                        ? formatBytes(server.metrics.ram_used)
                        : '0 B'}{' '}
                      / {formatBytes(server.ram_total)}
                    </span>
                  </div>
                  <div className="progress-bar">
                    <div
                      className="progress-bar-fill bg-green-500"
                      style={{
                        width: `${
                          server.metrics
                            ? (server.metrics.ram_used / server.ram_total) * 100
                            : 0
                        }%`,
                      }}
                    />
                  </div>
                </div>

                <div>
                  <div className="flex items-center justify-between mb-1">
                    <div className="flex items-center gap-2">
                      <HardDrive size={12} className="text-dark-400" />
                      <span className="text-xs text-dark-400">Disk</span>
                    </div>
                    <span className="text-xs text-dark-300">
                      {server.metrics
                        ? formatBytes(server.metrics.disk_used)
                        : '0 B'}{' '}
                      / {formatBytes(server.disk_total)}
                    </span>
                  </div>
                  <div className="progress-bar">
                    <div
                      className="progress-bar-fill bg-purple-500"
                      style={{
                        width: `${
                          server.metrics
                            ? (server.metrics.disk_used / server.disk_total) * 100
                            : 0
                        }%`,
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
