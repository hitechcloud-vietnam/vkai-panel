'use client';

import { useState } from 'react';
import { Activity, RefreshCw } from 'lucide-react';

const LOG_TABS = [
  { id: 'audit', label: 'Audit Logs' },
  { id: 'system', label: 'System Logs' },
  { id: 'access', label: 'Access Logs' },
];

export default function LogsPage() {
  const [activeTab, setActiveTab] = useState('audit');

  const activeLabel = LOG_TABS.find((t) => t.id === activeTab)?.label || 'Logs';

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Logs</h1>
          <p className="mt-1 text-sm text-gray-600">View audit logs and system logs</p>
        </div>
        <button
          type="button"
          className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2"
        >
          <RefreshCw size={16} />
          Refresh
        </button>
      </div>

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <nav className="flex gap-6" aria-label="Log categories">
          {LOG_TABS.map((tab) => (
            <button
              key={tab.id}
              type="button"
              onClick={() => setActiveTab(tab.id)}
              aria-current={activeTab === tab.id ? 'page' : undefined}
              className={`-mb-px border-b-2 px-1 py-3 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 ${
                activeTab === tab.id
                  ? 'border-blue-600 text-blue-700'
                  : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-900'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
        <div className="border-b border-gray-200 px-5 py-4">
          <h2 className="text-sm font-semibold text-gray-900">{activeLabel}</h2>
        </div>
        <div className="px-5 py-12 text-center">
          <Activity className="mx-auto text-gray-300" size={40} />
          <h3 className="mt-3 text-sm font-semibold text-gray-900">No logs</h3>
          <p className="mt-1 text-sm text-gray-600">
            Logs will appear here as actions are performed
          </p>
        </div>
      </div>
    </div>
  );
}
