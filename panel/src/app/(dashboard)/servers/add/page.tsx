'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { Server, ArrowLeft, Loader2 } from 'lucide-react';
import Link from 'next/link';
import { api } from '@/services/api';
import { brand } from '@/lib/brand';

export default function AddServerPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [formData, setFormData] = useState({
    name: '',
    hostname: '',
    ip_address: '',
    os: 'Ubuntu 22.04',
    ssh_port: 22,
    notes: '',
  });

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) => {
    setFormData({ ...formData, [e.target.name]: e.target.value });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');

    try {
      const response = await api.post('/api/v1/servers', formData);
      const server = response?.data?.data;
      if (server?.id) {
        router.push(`/servers/${server.id}`);
      } else {
        router.push('/servers');
      }
    } catch (err: any) {
      setError(err?.response?.data?.error || err?.message || 'Failed to add server');
    } finally {
      setLoading(false);
    }
  };

  const inputClass =
    'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus:outline-none';
  const labelClass = 'block text-sm font-medium text-gray-700 mb-1.5';

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      {/* Page header */}
      <div className="flex items-center gap-3">
        <Link
          href="/servers"
          aria-label="Back to servers"
          className="rounded-md border border-gray-300 bg-white p-2 text-gray-600 hover:bg-gray-50 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
        >
          <ArrowLeft size={18} aria-hidden="true" />
        </Link>
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Add Server</h1>
          <p className="text-sm text-gray-600 mt-1">Register a new server to manage</p>
        </div>
      </div>

      {/* Form */}
      <div className="bg-white border border-gray-200 rounded-lg shadow-sm">
        <div className="px-5 py-4 border-b border-gray-200">
          <h2 className="text-sm font-semibold text-gray-900">Server details</h2>
        </div>

        <div className="px-5 py-5">
          {error && (
            <div
              role="alert"
              className="mb-5 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
            >
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-5">
            <div>
              <label htmlFor="server-name" className={labelClass}>
                Server Name *
              </label>
              <input
                id="server-name"
                type="text"
                name="name"
                value={formData.name}
                onChange={handleChange}
                className={inputClass}
                placeholder="e.g., Production Web Server"
                required
              />
            </div>

            <div>
              <label htmlFor="server-hostname" className={labelClass}>
                Hostname *
              </label>
              <input
                id="server-hostname"
                type="text"
                name="hostname"
                value={formData.hostname}
                onChange={handleChange}
                className={inputClass}
                placeholder="e.g., web01.example.com"
                required
              />
            </div>

            <div>
              <label htmlFor="server-ip" className={labelClass}>
                IP Address *
              </label>
              <input
                id="server-ip"
                type="text"
                name="ip_address"
                value={formData.ip_address}
                onChange={handleChange}
                className={inputClass}
                placeholder="e.g., 192.168.1.100"
                required
              />
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <label htmlFor="server-os" className={labelClass}>
                  Operating System
                </label>
                <select
                  id="server-os"
                  name="os"
                  value={formData.os}
                  onChange={handleChange}
                  className={inputClass}
                >
                  <option value="Ubuntu 22.04">Ubuntu 22.04</option>
                  <option value="Ubuntu 24.04">Ubuntu 24.04</option>
                  <option value="Debian 11">Debian 11</option>
                  <option value="Debian 12">Debian 12</option>
                  <option value="CentOS 7">CentOS 7</option>
                  <option value="AlmaLinux 9">AlmaLinux 9</option>
                  <option value="Rocky Linux 9">Rocky Linux 9</option>
                </select>
              </div>

              <div>
                <label htmlFor="server-ssh-port" className={labelClass}>
                  SSH Port
                </label>
                <input
                  id="server-ssh-port"
                  type="number"
                  name="ssh_port"
                  value={formData.ssh_port}
                  onChange={handleChange}
                  className={inputClass}
                  min="1"
                  max="65535"
                />
              </div>
            </div>

            <div>
              <label htmlFor="server-notes" className={labelClass}>
                Notes
              </label>
              <textarea
                id="server-notes"
                name="notes"
                value={formData.notes}
                onChange={handleChange}
                className={inputClass}
                rows={3}
                placeholder="Optional notes about this server..."
              />
            </div>

            {/* Agent Installation Instructions */}
            <div className="rounded-md border border-gray-200 bg-gray-50 p-4">
              <h3 className="text-sm font-semibold text-gray-900 mb-1">Agent Installation</h3>
              <p className="text-sm text-gray-600 mb-3">
                After adding the server, install the {brand.productName} agent on the target machine:
              </p>
              <div className="overflow-x-auto rounded-md border border-gray-200 bg-white p-3 font-mono text-xs text-gray-700">
                <p className="text-gray-500"># Download and install agent</p>
                <p>curl -sSL https://install.vkai.vn/agent.sh | bash</p>
                <p className="mt-2 text-gray-500"># Or manually:</p>
                <p>wget https://install.vkai.vn/agent/latest/vkai-agent</p>
                <p>chmod +x vkai-agent</p>
                <p>sudo ./vkai-agent install</p>
              </div>
            </div>

            <div className="flex items-center justify-end gap-3 pt-4 border-t border-gray-200">
              <Link
                href="/servers"
                className="inline-flex items-center rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                Cancel
              </Link>
              <button
                type="submit"
                disabled={loading}
                className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {loading ? (
                  <Loader2 className="animate-spin" size={16} aria-hidden="true" />
                ) : (
                  <Server size={16} aria-hidden="true" />
                )}
                {loading ? 'Adding...' : 'Add Server'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  );
}
