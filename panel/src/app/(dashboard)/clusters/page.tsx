'use client';

import { useState, useEffect } from 'react';
import { Server, Plus, RefreshCw, Settings, Trash2, Edit } from 'lucide-react';
import { clusterApi } from '@/services/api';

interface Cluster {
  id: string;
  name: string;
  description: string;
  type: string;
  status: string;
  node_count: number;
  created_at: string;
}

interface ClusterNode {
  id: string;
  cluster_id: string;
  server_id: string;
  role: string;
  status: string;
  ip_address: string;
  last_heartbeat: string;
}

interface LoadBalancer {
  id: string;
  name: string;
  type: string;
  algorithm: string;
  status: string;
  listen_port: number;
  ssl_enabled: boolean;
}

interface HAPair {
  id: string;
  name: string;
  primary_server_id: string;
  secondary_server_id: string;
  virtual_ip: string;
  status: string;
  failover_mode: string;
  last_failover: string | null;
}

const CARD = 'bg-white border border-gray-200 rounded-lg shadow-sm';
const CARD_HEADER = 'px-5 py-4 border-b border-gray-200';
const TH = 'px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500';
const TD = 'px-4 py-3 text-sm text-gray-700';
const ROW = 'border-b border-gray-100 hover:bg-gray-50';
const BADGE = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium';
const BTN_PRIMARY =
  'inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1';
const BTN_SECONDARY =
  'inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1';
const BTN_SMALL =
  'inline-flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500';
const BTN_SMALL_DANGER =
  'inline-flex items-center gap-1 rounded-md border border-red-300 bg-white px-2.5 py-1.5 text-xs font-medium text-red-700 hover:bg-red-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500';

const TABS = [
  { id: 'clusters', label: 'Clusters' },
  { id: 'nodes', label: 'Nodes' },
  { id: 'load-balancers', label: 'Load Balancers' },
  { id: 'ha-pairs', label: 'HA Pairs' },
];

function shortId(value?: string): string {
  if (!value) return '—';
  return value.length > 8 ? `${value.slice(0, 8)}...` : value;
}

function formatDate(value?: string | null): string {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleDateString();
}

function formatDateTime(value?: string | null): string {
  if (!value) return 'Never';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return 'Never';
  return d.toLocaleString();
}

export default function ClustersPage() {
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [nodes, setNodes] = useState<ClusterNode[]>([]);
  const [loadBalancers, setLoadBalancers] = useState<LoadBalancer[]>([]);
  const [haPairs, setHAPairs] = useState<HAPair[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [activeTab, setActiveTab] = useState('clusters');

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      setLoading(true);
      setError('');
      const [clustersRes, lbsRes, haRes] = await Promise.all([
        clusterApi.getClusters(),
        clusterApi.getLoadBalancers(),
        clusterApi.getHAPairs(),
      ]);
      setClusters(Array.isArray(clustersRes?.data?.clusters) ? clustersRes.data.clusters : []);
      setLoadBalancers(Array.isArray(lbsRes?.data?.load_balancers) ? lbsRes.data.load_balancers : []);
      setHAPairs(Array.isArray(haRes?.data?.ha_pairs) ? haRes.data.ha_pairs : []);
    } catch (error) {
      console.error('Failed to fetch cluster data:', error);
      setError('Unable to load cluster data. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  const fetchNodes = async (clusterId: string) => {
    try {
      const res = await clusterApi.getNodes(clusterId);
      setNodes(Array.isArray(res?.data?.nodes) ? res.data.nodes : []);
      setActiveTab('nodes');
    } catch (error) {
      console.error('Failed to fetch nodes:', error);
      setError('Unable to load cluster nodes. Please try again.');
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'active':
        return 'bg-emerald-50 text-emerald-700';
      case 'creating':
      case 'joining':
      case 'configuring':
        return 'bg-brand-50 text-brand-700';
      case 'inactive':
      case 'maintenance':
        return 'bg-amber-50 text-amber-700';
      case 'failed':
      case 'error':
      case 'degraded':
        return 'bg-red-50 text-red-700';
      default:
        return 'bg-gray-100 text-gray-700';
    }
  };

  const getTypeLabel = (type: string) => {
    const labels: Record<string, string> = {
      'active-active': 'Active-Active',
      'active-passive': 'Active-Passive',
      'load-balanced': 'Load Balanced',
    };
    return labels[type] || type || '—';
  };

  if (loading) {
    return (
      <div className={`${CARD} px-5 py-12 text-center text-sm text-gray-500`}>Loading…</div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Clusters &amp; HA</h1>
          <p className="mt-1 text-sm text-gray-600">Manage clusters, load balancers, and high availability</p>
        </div>
        <div className="flex items-center gap-2">
          <button type="button" onClick={fetchData} className={BTN_SECONDARY}>
            <RefreshCw className="h-4 w-4" />
            Refresh
          </button>
          <button type="button" className={BTN_PRIMARY}>
            <Plus className="h-4 w-4" />
            Create Cluster
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="Cluster sections">
          {TABS.map((tab) => (
            <button
              key={tab.id}
              type="button"
              aria-current={activeTab === tab.id ? 'page' : undefined}
              onClick={() => setActiveTab(tab.id)}
              className={`border-b-2 px-1 pb-3 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 ${
                activeTab === tab.id
                  ? 'border-brand-600 text-brand-700'
                  : 'border-transparent text-gray-600 hover:border-gray-300 hover:text-gray-900'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {activeTab === 'clusters' && (
        clusters.length === 0 ? (
          <div className={`${CARD} px-5 py-12 text-center`}>
            <Server className="mx-auto mb-3 text-gray-400" size={36} />
            <p className="text-sm text-gray-600">No clusters configured yet.</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {clusters.map((cluster) => (
              <div key={cluster.id} className={CARD}>
                <div className={`${CARD_HEADER} flex items-center justify-between gap-3`}>
                  <h2 className="text-sm font-semibold text-gray-900">{cluster.name}</h2>
                  <span className={`${BADGE} ${getStatusColor(cluster.status)}`}>{cluster.status || 'unknown'}</span>
                </div>
                <div className="p-5">
                  <p className="mb-4 text-sm text-gray-600">{cluster.description || 'No description'}</p>
                  <div className="space-y-2">
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Type</span>
                      <span className="text-sm font-medium text-gray-900">{getTypeLabel(cluster.type)}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Nodes</span>
                      <span className="text-sm font-medium text-gray-900">{cluster.node_count ?? 0}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Created</span>
                      <span className="text-sm text-gray-700" suppressHydrationWarning>{formatDate(cluster.created_at)}</span>
                    </div>
                  </div>
                  <div className="mt-4 flex justify-end gap-2">
                    <button type="button" className={BTN_SMALL} onClick={() => fetchNodes(cluster.id)}>
                      <Server className="h-4 w-4" />
                      Nodes
                    </button>
                    <button type="button" className={BTN_SMALL} aria-label={`Edit cluster ${cluster.name}`}>
                      <Edit className="h-4 w-4" />
                    </button>
                    <button type="button" className={BTN_SMALL_DANGER} aria-label={`Delete cluster ${cluster.name}`}>
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )
      )}

      {activeTab === 'nodes' && (
        <div className={CARD}>
          <div className={CARD_HEADER}>
            <h2 className="text-sm font-semibold text-gray-900">Cluster Nodes</h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 border-b border-gray-200">
                <tr>
                  <th className={TH}>Node ID</th>
                  <th className={TH}>Server</th>
                  <th className={TH}>Role</th>
                  <th className={TH}>Status</th>
                  <th className={TH}>IP Address</th>
                  <th className={TH}>Last Heartbeat</th>
                </tr>
              </thead>
              <tbody>
                {nodes.length === 0 ? (
                  <tr><td colSpan={6} className="px-4 py-10 text-center text-sm text-gray-500">No nodes loaded. Open a cluster to view its nodes.</td></tr>
                ) : nodes.map((node) => (
                  <tr key={node.id} className={ROW}>
                    <td className={`${TD} font-mono`}>{shortId(node.id)}</td>
                    <td className={`${TD} font-mono`}>{shortId(node.server_id)}</td>
                    <td className="px-4 py-3">
                      <span className={`${BADGE} bg-gray-100 text-gray-700`}>{node.role || '—'}</span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`${BADGE} ${getStatusColor(node.status)}`}>{node.status || 'unknown'}</span>
                    </td>
                    <td className={`${TD} font-mono`}>{node.ip_address || '—'}</td>
                    <td className={TD} suppressHydrationWarning>{formatDateTime(node.last_heartbeat)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {activeTab === 'load-balancers' && (
        loadBalancers.length === 0 ? (
          <div className={`${CARD} px-5 py-12 text-center text-sm text-gray-600`}>No load balancers configured yet.</div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {loadBalancers.map((lb) => (
              <div key={lb.id} className={CARD}>
                <div className={`${CARD_HEADER} flex items-center justify-between gap-3`}>
                  <h2 className="text-sm font-semibold text-gray-900">{lb.name}</h2>
                  <span className={`${BADGE} ${getStatusColor(lb.status)}`}>{lb.status || 'unknown'}</span>
                </div>
                <div className="p-5">
                  <div className="space-y-2">
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Type</span>
                      <span className="text-sm font-medium text-gray-900">{lb.type || '—'}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Algorithm</span>
                      <span className="text-sm font-medium text-gray-900">{lb.algorithm || '—'}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Port</span>
                      <span className="text-sm font-medium text-gray-900">{lb.listen_port ?? '—'}</span>
                    </div>
                    <div className="flex items-center justify-between">
                      <span className="text-sm text-gray-500">SSL</span>
                      <span className={`${BADGE} ${lb.ssl_enabled ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-700'}`}>
                        {lb.ssl_enabled ? 'Enabled' : 'Disabled'}
                      </span>
                    </div>
                  </div>
                  <div className="mt-4 flex justify-end gap-2">
                    <button type="button" className={BTN_SMALL} aria-label={`Configure load balancer ${lb.name}`}>
                      <Settings className="h-4 w-4" />
                    </button>
                    <button type="button" className={BTN_SMALL_DANGER} aria-label={`Delete load balancer ${lb.name}`}>
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )
      )}

      {activeTab === 'ha-pairs' && (
        haPairs.length === 0 ? (
          <div className={`${CARD} px-5 py-12 text-center text-sm text-gray-600`}>No HA pairs configured yet.</div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {haPairs.map((ha) => (
              <div key={ha.id} className={CARD}>
                <div className={`${CARD_HEADER} flex items-center justify-between gap-3`}>
                  <h2 className="text-sm font-semibold text-gray-900">{ha.name}</h2>
                  <span className={`${BADGE} ${getStatusColor(ha.status)}`}>{ha.status || 'unknown'}</span>
                </div>
                <div className="p-5">
                  <div className="space-y-2">
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Primary</span>
                      <span className="font-mono text-sm text-gray-900">{shortId(ha.primary_server_id)}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Secondary</span>
                      <span className="font-mono text-sm text-gray-900">{shortId(ha.secondary_server_id)}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Virtual IP</span>
                      <span className="font-mono text-sm text-gray-900">{ha.virtual_ip || 'N/A'}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Failover Mode</span>
                      <span className="text-sm font-medium text-gray-900">{ha.failover_mode || '—'}</span>
                    </div>
                    {ha.last_failover && (
                      <div className="flex justify-between">
                        <span className="text-sm text-gray-500">Last Failover</span>
                        <span className="text-sm text-gray-700" suppressHydrationWarning>{formatDateTime(ha.last_failover)}</span>
                      </div>
                    )}
                  </div>
                  <div className="mt-4 flex justify-end gap-2">
                    <button type="button" className={BTN_SMALL} aria-label={`Configure HA pair ${ha.name}`}>
                      <Settings className="h-4 w-4" />
                    </button>
                    <button
                      type="button"
                      className="inline-flex items-center gap-1 rounded-md bg-red-600 px-2.5 py-1.5 text-xs font-medium text-white hover:bg-red-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
                    >
                      Failover
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )
      )}
    </div>
  );
}
