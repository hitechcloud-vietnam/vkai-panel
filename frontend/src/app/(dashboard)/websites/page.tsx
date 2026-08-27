'use client';

import { useEffect, useState } from 'react';
import { api } from '@/services/api';

interface Website {
  id: string;
  name: string;
  domain: string;
  type: string;
  status: string;
  server_id: string;
  php_version: string;
  ssl_enabled: boolean;
  created_at: string;
}

export default function WebsitesPage() {
  const [websites, setWebsites] = useState<Website[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadWebsites();
  }, []);

  const loadWebsites = async () => {
    try {
      const response = await api.get('/api/v1/websites');
      setWebsites(response.data.data || []);
    } catch (error) {
      console.error('Failed to load websites:', error);
    } finally {
      setLoading(false);
    }
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'active':
        return 'badge-success';
      case 'inactive':
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
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Websites</h1>
          <p className="text-dark-400 mt-1">Manage your websites and applications</p>
        </div>
        <button className="btn btn-primary">Add Website</button>
      </div>

      {websites.length === 0 ? (
        <div className="text-center py-16">
          <h3 className="text-xl font-medium text-dark-300">No websites yet</h3>
          <p className="mt-2 text-dark-500">Add your first website to get started</p>
        </div>
      ) : (
        <div className="card">
          <table>
            <thead>
              <tr>
                <th>Domain</th>
                <th>Type</th>
                <th>Status</th>
                <th>PHP</th>
                <th>SSL</th>
                <th>Created</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {websites.map((site) => (
                <tr key={site.id}>
                  <td className="font-medium">{site.domain}</td>
                  <td>{site.type}</td>
                  <td>
                    <span className={`badge ${getStatusBadge(site.status)}`}>
                      {site.status}
                    </span>
                  </td>
                  <td>{site.php_version || 'N/A'}</td>
                  <td>{site.ssl_enabled ? '✓' : '✗'}</td>
                  <td>{new Date(site.created_at).toLocaleDateString()}</td>
                  <td>
                    <button className="btn btn-secondary btn-sm">Manage</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
