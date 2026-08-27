'use client';

import { useEffect, useState } from 'react';
import { Globe, Plus, Check, Minus } from 'lucide-react';
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
  const [error, setError] = useState('');
  const [createdDates, setCreatedDates] = useState<Record<string, string>>({});

  useEffect(() => {
    loadWebsites();
  }, []);

  // Format dates on the client only, to avoid SSR/CSR hydration drift.
  useEffect(() => {
    const map: Record<string, string> = {};
    websites.forEach((site) => {
      if (!site?.id) return;
      if (site?.created_at) {
        const d = new Date(site.created_at);
        map[site.id] = Number.isNaN(d.getTime()) ? '-' : d.toLocaleDateString();
      } else {
        map[site.id] = '-';
      }
    });
    setCreatedDates(map);
  }, [websites]);

  const loadWebsites = async () => {
    setLoading(true);
    setError('');
    try {
      const response = await api.get('/api/v1/websites');
      setWebsites(Array.isArray(response?.data?.data) ? response.data.data : []);
    } catch (err: any) {
      console.error('Failed to load websites:', err);
      setWebsites([]);
      setError(err?.response?.data?.error || err?.message || 'Failed to load websites');
    } finally {
      setLoading(false);
    }
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'active':
        return 'bg-emerald-50 text-emerald-700';
      case 'inactive':
        return 'bg-red-50 text-red-700';
      default:
        return 'bg-amber-50 text-amber-700';
    }
  };

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
          <h1 className="text-xl font-semibold text-gray-900">Websites</h1>
          <p className="text-sm text-gray-600 mt-1">Manage your websites and applications</p>
        </div>
        <button
          type="button"
          className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
        >
          <Plus size={16} aria-hidden="true" />
          Add Website
        </button>
      </div>

      {error && (
        <div
          role="alert"
          className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
        >
          {error}
        </div>
      )}

      <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
        <div className="px-5 py-4 border-b border-gray-200">
          <h2 className="text-sm font-semibold text-gray-900">All websites</h2>
        </div>

        {websites.length === 0 ? (
          <div className="text-center px-6 py-14">
            <Globe className="mx-auto text-gray-300" size={40} aria-hidden="true" />
            <h3 className="mt-4 text-sm font-semibold text-gray-900">No websites yet</h3>
            <p className="mt-1 text-sm text-gray-600">Add your first website to get started</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[800px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Domain</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Type</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Status</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">PHP</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">SSL</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Created</th>
                  <th className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">Actions</th>
                </tr>
              </thead>
              <tbody>
                {websites.map((site) => (
                  <tr key={site?.id} className="border-b border-gray-100 hover:bg-gray-50">
                    <td className="px-4 py-3 text-sm font-medium text-gray-900">
                      {site?.domain || '-'}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700">{site?.type || '-'}</td>
                    <td className="px-4 py-3 text-sm">
                      <span
                        className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${getStatusBadge(
                          site?.status || ''
                        )}`}
                      >
                        {site?.status || 'unknown'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700">
                      {site?.php_version || 'N/A'}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700">
                      {site?.ssl_enabled ? (
                        <span className="inline-flex items-center gap-1 rounded-md bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
                          <Check size={12} aria-hidden="true" />
                          Enabled
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600">
                          <Minus size={12} aria-hidden="true" />
                          Off
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700" suppressHydrationWarning>
                      {createdDates[site?.id] || '-'}
                    </td>
                    <td className="px-4 py-3 text-sm">
                      <button
                        type="button"
                        className="inline-flex items-center rounded-md border border-gray-300 bg-white px-2.5 py-1 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                      >
                        Manage
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
