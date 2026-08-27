'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { Server, ArrowLeft, Loader2 } from 'lucide-react';
import Link from 'next/link';
import { api } from '@/services/api';

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
      const server = response.data.data;
      router.push(`/servers/${server.id}`);
    } catch (err: any) {
      setError(err.response?.data?.error || 'Failed to add server');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Link href="/servers" className="p-2 hover:bg-dark-800 rounded-lg transition-colors">
          <ArrowLeft size={20} className="text-dark-400" />
        </Link>
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Add Server</h1>
          <p className="text-dark-400 mt-1">Register a new server to manage</p>
        </div>
      </div>

      {/* Form */}
      <div className="card">
        {error && (
          <div className="mb-6 p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-red-400 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-6">
          <div>
            <label className="block text-sm font-medium text-dark-300 mb-2">
              Server Name *
            </label>
            <input
              type="text"
              name="name"
              value={formData.name}
              onChange={handleChange}
              className="input"
              placeholder="e.g., Production Web Server"
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-dark-300 mb-2">
              Hostname *
            </label>
            <input
              type="text"
              name="hostname"
              value={formData.hostname}
              onChange={handleChange}
              className="input"
              placeholder="e.g., web01.example.com"
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-dark-300 mb-2">
              IP Address *
            </label>
            <input
              type="text"
              name="ip_address"
              value={formData.ip_address}
              onChange={handleChange}
              className="input"
              placeholder="e.g., 192.168.1.100"
              required
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-dark-300 mb-2">
                Operating System
              </label>
              <select
                name="os"
                value={formData.os}
                onChange={handleChange}
                className="input"
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
              <label className="block text-sm font-medium text-dark-300 mb-2">
                SSH Port
              </label>
              <input
                type="number"
                name="ssh_port"
                value={formData.ssh_port}
                onChange={handleChange}
                className="input"
                min="1"
                max="65535"
              />
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-dark-300 mb-2">
              Notes
            </label>
            <textarea
              name="notes"
              value={formData.notes}
              onChange={handleChange}
              className="input"
              rows={3}
              placeholder="Optional notes about this server..."
            />
          </div>

          {/* Agent Installation Instructions */}
          <div className="p-4 bg-dark-800 rounded-lg border border-dark-600">
            <h3 className="text-sm font-medium text-dark-200 mb-2">
              Agent Installation
            </h3>
            <p className="text-sm text-dark-400 mb-3">
              After adding the server, install the vKAI Agent on the target machine:
            </p>
            <div className="bg-dark-900 rounded-lg p-3 font-mono text-sm text-dark-300">
              <p># Download and install agent</p>
              <p>curl -sSL https://get.vkai.cloud/agent | bash</p>
              <p className="mt-2"># Or manually:</p>
              <p>wget https://releases.vkai.cloud/agent/latest/vkaid</p>
              <p>chmod +x vkaid</p>
              <p>./vkaid install</p>
            </div>
          </div>

          <div className="flex items-center justify-end gap-3 pt-4 border-t border-dark-700">
            <Link href="/servers" className="btn btn-secondary">
              Cancel
            </Link>
            <button type="submit" disabled={loading} className="btn btn-primary">
              {loading ? (
                <Loader2 className="animate-spin" size={16} />
              ) : (
                <Server size={16} />
              )}
              Add Server
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
