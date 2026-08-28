'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { ArrowLeft, Loader2, Server } from 'lucide-react';

import { serverApi, unwrap } from '@/services/api';
import { brand } from '@/lib/brand';
import type { ManagedServer } from '@/types/server';
import { errorMessage } from '@/lib/apiError';

/**
 * Registering an extra machine.
 *
 * The panel already manages the machine it was installed on, so nothing on this
 * screen is a precondition for using the product - it is how a fleet grows. The
 * form sends exactly what POST /api/v1/servers accepts: there is no `name`
 * column on a server, the hostname is the name.
 */

const INPUT_CLASS =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500';
const LABEL_CLASS = 'mb-1.5 block text-sm font-medium text-gray-700';

export default function AddServerPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [form, setForm] = useState({
    hostname: '',
    ip_address: '',
    ssh_port: '22',
    location: '',
    tags: '',
  });

  const update = (field: keyof typeof form, value: string) =>
    setForm((prev) => ({ ...prev, [field]: value }));

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');

    const tags = form.tags
      .split(',')
      .map((tag) => tag.trim())
      .filter(Boolean);

    try {
      const response = await serverApi.create({
        hostname: form.hostname.trim(),
        ip_address: form.ip_address.trim(),
        ssh_port: Number(form.ssh_port) || 22,
        location: form.location.trim(),
        tags,
      });
      const server = unwrap<ManagedServer>(response, null);
      router.push(server?.id ? `/servers/${server.id}` : '/servers');
    } catch (err: any) {
      setError(
        errorMessage(err, 'Failed to add server')
      );
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="mx-auto max-w-2xl space-y-6">
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
          <p className="mt-1 text-sm text-gray-600">
            Register a second machine. The machine this panel runs on is already managed.
          </p>
        </div>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
        <div className="border-b border-gray-200 px-5 py-4">
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
              <label htmlFor="server-hostname" className={LABEL_CLASS}>
                Hostname <span className="text-red-600">*</span>
              </label>
              <input
                id="server-hostname"
                type="text"
                value={form.hostname}
                onChange={(e) => update('hostname', e.target.value)}
                className={INPUT_CLASS}
                placeholder="web01.example.vn"
                required
              />
              <p className="mt-1 text-xs text-gray-500">
                A server has no separate display name: the hostname is how it appears
                everywhere in the panel.
              </p>
            </div>

            <div>
              <label htmlFor="server-ip" className={LABEL_CLASS}>
                IP address <span className="text-red-600">*</span>
              </label>
              <input
                id="server-ip"
                type="text"
                value={form.ip_address}
                onChange={(e) => update('ip_address', e.target.value)}
                className={INPUT_CLASS}
                placeholder="192.0.2.10"
                required
              />
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label htmlFor="server-ssh-port" className={LABEL_CLASS}>
                  SSH port
                </label>
                <input
                  id="server-ssh-port"
                  type="number"
                  value={form.ssh_port}
                  onChange={(e) => update('ssh_port', e.target.value)}
                  className={INPUT_CLASS}
                  min={1}
                  max={65535}
                />
              </div>
              <div>
                <label htmlFor="server-location" className={LABEL_CLASS}>
                  Location
                </label>
                <input
                  id="server-location"
                  type="text"
                  value={form.location}
                  onChange={(e) => update('location', e.target.value)}
                  className={INPUT_CLASS}
                  placeholder="Ha Noi"
                />
              </div>
            </div>

            <div>
              <label htmlFor="server-tags" className={LABEL_CLASS}>
                Tags
              </label>
              <input
                id="server-tags"
                type="text"
                value={form.tags}
                onChange={(e) => update('tags', e.target.value)}
                className={INPUT_CLASS}
                placeholder="production, web"
              />
              <p className="mt-1 text-xs text-gray-500">Separated by commas.</p>
            </div>

            <div className="rounded-md border border-gray-200 bg-gray-50 p-4">
              <h3 className="mb-1 text-sm font-semibold text-gray-900">
                Then connect the agent
              </h3>
              <p className="mb-3 text-sm text-gray-600">
                Registering the machine here records it. It starts reporting once the{' '}
                {brand.productName} agent on that machine has enrolled with this panel: the
                agent generates its own key, collects a certificate over a single-use
                enrolment token, and connects on mutual TLS.
              </p>
              <div className="overflow-x-auto rounded-md border border-gray-200 bg-white p-3 font-mono text-xs text-gray-700">
                <p className="text-gray-500"># On the new machine</p>
                <p>curl -sSL https://install.vkai.vn/agent.sh | bash</p>
                <p className="mt-2 text-gray-500"># Then start it with this panel and a fresh token</p>
                <p>VKAI_PANEL_URL=https://panel.example.vn \</p>
                <p>VKAI_AGENT_ENROLMENT_TOKEN=vkai-enrol.v1.... \</p>
                <p>&nbsp;&nbsp;sudo systemctl start vkai-agent</p>
              </div>
              <p className="mt-3 text-xs text-gray-500">
                The token is single use and expires in 30 minutes.
              </p>
            </div>

            <div className="flex items-center justify-end gap-3 border-t border-gray-200 pt-4">
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
