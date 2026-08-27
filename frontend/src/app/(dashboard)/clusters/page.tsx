'use client';

import { useState, useEffect } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
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

export default function ClustersPage() {
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [nodes, setNodes] = useState<ClusterNode[]>([]);
  const [loadBalancers, setLoadBalancers] = useState<LoadBalancer[]>([]);
  const [haPairs, setHAPairs] = useState<HAPair[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState('clusters');

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      setLoading(true);
      const [clustersRes, lbsRes, haRes] = await Promise.all([
        clusterApi.getClusters(),
        clusterApi.getLoadBalancers(),
        clusterApi.getHAPairs(),
      ]);
      setClusters(clustersRes.data.clusters || []);
      setLoadBalancers(lbsRes.data.load_balancers || []);
      setHAPairs(haRes.data.ha_pairs || []);
    } catch (error) {
      console.error('Failed to fetch cluster data:', error);
    } finally {
      setLoading(false);
    }
  };

  const fetchNodes = async (clusterId: string) => {
    try {
      const res = await clusterApi.getNodes(clusterId);
      setNodes(res.data.nodes || []);
    } catch (error) {
      console.error('Failed to fetch nodes:', error);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'active':
        return 'bg-green-100 text-green-800';
      case 'creating':
      case 'joining':
      case 'configuring':
        return 'bg-blue-100 text-blue-800';
      case 'inactive':
      case 'maintenance':
        return 'bg-yellow-100 text-yellow-800';
      case 'failed':
      case 'error':
      case 'degraded':
        return 'bg-red-100 text-red-800';
      default:
        return 'bg-gray-100 text-gray-800';
    }
  };

  const getTypeLabel = (type: string) => {
    const labels: Record<string, string> = {
      'active-active': 'Active-Active',
      'active-passive': 'Active-Passive',
      'load-balanced': 'Load Balanced',
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
          <h1 className="text-3xl font-bold">Clusters & HA</h1>
          <p className="text-gray-500">Manage clusters, load balancers, and high availability</p>
        </div>
        <div className="flex space-x-2">
          <Button variant="outline" onClick={fetchData}>
            <RefreshCw className="h-4 w-4 mr-2" />
            Refresh
          </Button>
          <Button>
            <Plus className="h-4 w-4 mr-2" />
            Create Cluster
          </Button>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="clusters">Clusters</TabsTrigger>
          <TabsTrigger value="nodes">Nodes</TabsTrigger>
          <TabsTrigger value="load-balancers">Load Balancers</TabsTrigger>
          <TabsTrigger value="ha-pairs">HA Pairs</TabsTrigger>
        </TabsList>

        <TabsContent value="clusters" className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {clusters.map((cluster) => (
              <Card key={cluster.id}>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-lg">{cluster.name}</CardTitle>
                    <Badge className={getStatusColor(cluster.status)}>{cluster.status}</Badge>
                  </div>
                </CardHeader>
                <CardContent>
                  <p className="text-sm text-gray-500 mb-4">{cluster.description}</p>
                  <div className="space-y-2">
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Type</span>
                      <span className="text-sm font-medium">{getTypeLabel(cluster.type)}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Nodes</span>
                      <span className="text-sm font-medium">{cluster.node_count}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Created</span>
                      <span className="text-sm">
                        {new Date(cluster.created_at).toLocaleDateString()}
                      </span>
                    </div>
                  </div>
                  <div className="flex justify-end space-x-2 mt-4">
                    <Button variant="outline" size="sm" onClick={() => fetchNodes(cluster.id)}>
                      <Server className="h-4 w-4 mr-1" />
                      Nodes
                    </Button>
                    <Button variant="outline" size="sm">
                      <Edit className="h-4 w-4" />
                    </Button>
                    <Button variant="outline" size="sm">
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </TabsContent>

        <TabsContent value="nodes" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Cluster Nodes</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b">
                      <th className="text-left p-2">Node ID</th>
                      <th className="text-left p-2">Server</th>
                      <th className="text-left p-2">Role</th>
                      <th className="text-left p-2">Status</th>
                      <th className="text-left p-2">IP Address</th>
                      <th className="text-left p-2">Last Heartbeat</th>
                    </tr>
                  </thead>
                  <tbody>
                    {nodes.map((node) => (
                      <tr key={node.id} className="border-b">
                        <td className="p-2 font-mono text-sm">{node.id.slice(0, 8)}...</td>
                        <td className="p-2 font-mono text-sm">{node.server_id.slice(0, 8)}...</td>
                        <td className="p-2">
                          <Badge variant="outline">{node.role}</Badge>
                        </td>
                        <td className="p-2">
                          <Badge className={getStatusColor(node.status)}>{node.status}</Badge>
                        </td>
                        <td className="p-2 font-mono text-sm">{node.ip_address}</td>
                        <td className="p-2 text-sm text-gray-500">
                          {node.last_heartbeat
                            ? new Date(node.last_heartbeat).toLocaleString()
                            : 'Never'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="load-balancers" className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {loadBalancers.map((lb) => (
              <Card key={lb.id}>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-lg">{lb.name}</CardTitle>
                    <Badge className={getStatusColor(lb.status)}>{lb.status}</Badge>
                  </div>
                </CardHeader>
                <CardContent>
                  <div className="space-y-2">
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Type</span>
                      <span className="text-sm font-medium">{lb.type}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Algorithm</span>
                      <span className="text-sm font-medium">{lb.algorithm}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Port</span>
                      <span className="text-sm font-medium">{lb.listen_port}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">SSL</span>
                      <Badge variant={lb.ssl_enabled ? 'default' : 'secondary'}>
                        {lb.ssl_enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                    </div>
                  </div>
                  <div className="flex justify-end space-x-2 mt-4">
                    <Button variant="outline" size="sm">
                      <Settings className="h-4 w-4" />
                    </Button>
                    <Button variant="outline" size="sm">
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </TabsContent>

        <TabsContent value="ha-pairs" className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {haPairs.map((ha) => (
              <Card key={ha.id}>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-lg">{ha.name}</CardTitle>
                    <Badge className={getStatusColor(ha.status)}>{ha.status}</Badge>
                  </div>
                </CardHeader>
                <CardContent>
                  <div className="space-y-2">
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Primary</span>
                      <span className="text-sm font-mono">
                        {ha.primary_server_id.slice(0, 8)}...
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Secondary</span>
                      <span className="text-sm font-mono">
                        {ha.secondary_server_id.slice(0, 8)}...
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Virtual IP</span>
                      <span className="text-sm font-mono">{ha.virtual_ip || 'N/A'}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500">Failover Mode</span>
                      <span className="text-sm font-medium">{ha.failover_mode}</span>
                    </div>
                    {ha.last_failover && (
                      <div className="flex justify-between">
                        <span className="text-sm text-gray-500">Last Failover</span>
                        <span className="text-sm">
                          {new Date(ha.last_failover).toLocaleString()}
                        </span>
                      </div>
                    )}
                  </div>
                  <div className="flex justify-end space-x-2 mt-4">
                    <Button variant="outline" size="sm">
                      <Settings className="h-4 w-4" />
                    </Button>
                    <Button variant="destructive" size="sm">
                      Failover
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
}
