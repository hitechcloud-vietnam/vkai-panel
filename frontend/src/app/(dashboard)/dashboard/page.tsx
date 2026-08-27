'use client';

import { useEffect, useState } from 'react';
import {
  Server,
  Globe,
  Database,
  Activity,
  Cpu,
  MemoryStick,
  HardDrive,
  Network,
  ArrowUp,
  ArrowDown,
  AlertTriangle,
  CheckCircle,
  Clock,
} from 'lucide-react';
import { api } from '@/services/api';
import { useAuthStore } from '@/store/auth';

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

  useEffect(() => {
    loadDashboardData();
  }, []);

  const loadDashboardData = async () => {
    try {
      const [serversRes] = await Promise.all([
        api.get('/api/v1/servers'),
      ]);

      setServers(serversRes.data.data || []);
      setStats({
        servers: serversRes.data.data?.length || 0,
        websites: 0,
        databases: 0,
        sslCerts: 0,
      });
    } catch (error) {
      console.error('Failed to load dashboard data:', error);
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

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'online':
        return 'text-green-400';
      case 'offline':
        return 'text-red-400';
      default:
        return 'text-yellow-400';
    }
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

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Welcome */}
      <div>
        <h1 className="text-2xl font-bold text-dark-50">
          Welcome back, {user?.first_name || user?.username}
        </h1>
        <p className="text-dark-400 mt-1">
          Here&apos;s what&apos;s happening with your servers today.
        </p>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-dark-400">Servers</p>
              <p className="text-3xl font-bold text-dark-50">{stats.servers}</p>
            </div>
            <div className="p-3 bg-primary-600/10 rounded-lg">
              <Server className="text-primary-400" size={24} />
            </div>
          </div>
          <div className="mt-4 flex items-center gap-2 text-sm">
            <CheckCircle className="text-green-400" size={14} />
            <span className="text-green-400">
              {servers.filter((s) => s.status === 'online').length} online
            </span>
            {servers.filter((s) => s.status === 'offline').length > 0 && (
              <>
                <span className="text-dark-500">•</span>
                <AlertTriangle className="text-red-400" size={14} />
                <span className="text-red-400">
                  {servers.filter((s) => s.status === 'offline').length} offline
                </span>
              </>
            )}
          </div>
        </div>

        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-dark-400">Websites</p>
              <p className="text-3xl font-bold text-dark-50">{stats.websites}</p>
            </div>
            <div className="p-3 bg-green-600/10 rounded-lg">
              <Globe className="text-green-400" size={24} />
            </div>
          </div>
          <div className="mt-4 flex items-center gap-2 text-sm">
            <CheckCircle className="text-green-400" size={14} />
            <span className="text-green-400">All active</span>
          </div>
        </div>

        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-dark-400">Databases</p>
              <p className="text-3xl font-bold text-dark-50">{stats.databases}</p>
            </div>
            <div className="p-3 bg-purple-600/10 rounded-lg">
              <Database className="text-purple-400" size={24} />
            </div>
          </div>
          <div className="mt-4 flex items-center gap-2 text-sm">
            <Activity className="text-purple-400" size={14} />
            <span className="text-dark-400">MySQL, PostgreSQL, Redis</span>
          </div>
        </div>

        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-dark-400">SSL Certificates</p>
              <p className="text-3xl font-bold text-dark-50">{stats.sslCerts}</p>
            </div>
            <div className="p-3 bg-yellow-600/10 rounded-lg">
              <Activity className="text-yellow-400" size={24} />
            </div>
          </div>
          <div className="mt-4 flex items-center gap-2 text-sm">
            <CheckCircle className="text-green-400" size={14} />
            <span className="text-green-400">All valid</span>
          </div>
        </div>
      </div>

      {/* Server List */}
      <div className="card">
        <div className="card-header">
          <h2 className="text-lg font-semibold text-dark-50">Servers</h2>
          <button className="btn btn-primary btn-sm">
            <Server size={14} />
            Add Server
          </button>
        </div>

        {servers.length === 0 ? (
          <div className="text-center py-12">
            <Server className="mx-auto text-dark-600" size={48} />
            <h3 className="mt-4 text-lg font-medium text-dark-300">No servers yet</h3>
            <p className="mt-2 text-dark-500">
              Add your first server to get started with vKAI Panel.
            </p>
            <button className="mt-4 btn btn-primary">
              <Server size={16} />
              Add Your First Server
            </button>
          </div>
        ) : (
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Server</th>
                  <th>Status</th>
                  <th>IP Address</th>
                  <th>OS</th>
                  <th>CPU</th>
                  <th>RAM</th>
                  <th>Disk</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {servers.map((server) => (
                  <tr key={server.id}>
                    <td>
                      <div>
                        <p className="font-medium text-dark-100">{server.name}</p>
                        <p className="text-xs text-dark-500">{server.hostname}</p>
                      </div>
                    </td>
                    <td>
                      <span className={`badge ${getStatusBadge(server.status)}`}>
                        {server.status}
                      </span>
                    </td>
                    <td className="font-mono text-sm">{server.ip_address}</td>
                    <td className="text-sm">{server.os}</td>
                    <td>
                      <div className="flex items-center gap-2">
                        <Cpu size={14} className="text-dark-400" />
                        <span className="text-sm">
                          {server.metrics?.cpu_percent.toFixed(1) || 0}%
                        </span>
                      </div>
                    </td>
                    <td>
                      <div className="flex items-center gap-2">
                        <MemoryStick size={14} className="text-dark-400" />
                        <span className="text-sm">
                          {server.metrics
                            ? formatBytes(server.metrics.ram_used)
                            : '0 B'}{' '}
                          / {formatBytes(server.ram_total)}
                        </span>
                      </div>
                    </td>
                    <td>
                      <div className="flex items-center gap-2">
                        <HardDrive size={14} className="text-dark-400" />
                        <span className="text-sm">
                          {server.metrics
                            ? formatBytes(server.metrics.disk_used)
                            : '0 B'}{' '}
                          / {formatBytes(server.disk_total)}
                        </span>
                      </div>
                    </td>
                    <td>
                      <button className="btn btn-secondary btn-sm">View</button>
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
        <div className="card hover:border-primary-500/50 cursor-pointer transition-colors">
          <div className="flex items-center gap-4">
            <div className="p-3 bg-primary-600/10 rounded-lg">
              <Globe className="text-primary-400" size={24} />
            </div>
            <div>
              <h3 className="font-medium text-dark-100">Add Website</h3>
              <p className="text-sm text-dark-400">Deploy a new website or app</p>
            </div>
          </div>
        </div>

        <div className="card hover:border-primary-500/50 cursor-pointer transition-colors">
          <div className="flex items-center gap-4">
            <div className="p-3 bg-green-600/10 rounded-lg">
              <Database className="text-green-400" size={24} />
            </div>
            <div>
              <h3 className="font-medium text-dark-100">Create Database</h3>
              <p className="text-sm text-dark-400">MySQL, PostgreSQL, or Redis</p>
            </div>
          </div>
        </div>

        <div className="card hover:border-primary-500/50 cursor-pointer transition-colors">
          <div className="flex items-center gap-4">
            <div className="p-3 bg-purple-600/10 rounded-lg">
              <Activity className="text-purple-400" size={24} />
            </div>
            <div>
              <h3 className="font-medium text-dark-100">View Logs</h3>
              <p className="text-sm text-dark-400">Check server and app logs</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
