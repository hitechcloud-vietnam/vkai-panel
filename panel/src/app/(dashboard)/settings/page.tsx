'use client';

import { useState } from 'react';
import { Save } from 'lucide-react';
import { brand } from '@/lib/brand';
import PanelAccessSection from '@/components/settings/PanelAccessSection';

const TABS = [
  { id: 'general', label: 'General' },
  { id: 'panel-access', label: 'Panel access' },
  { id: 'security', label: 'Security' },
  { id: 'notifications', label: 'Notifications' },
  { id: 'backup', label: 'Backup' },
];

export default function SettingsPage() {
  const [activeTab, setActiveTab] = useState('general');

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Settings</h1>
          <p className="mt-1 text-sm text-gray-600">Configure panel settings</p>
        </div>
        {/* The panel access section saves through its own confirmation flow, so
            the page-level save button would be a second, silent path to the
            same settings. It is offered only where it applies. */}
        {activeTab === 'general' ? (
          <button
            type="button"
            className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1"
          >
            <Save size={16} />
            Save Changes
          </button>
        ) : null}
      </div>

      {/* Tabs */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="Settings sections">
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

      {activeTab === 'panel-access' ? <PanelAccessSection /> : null}

      {activeTab === 'general' ? (
        <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
          <div className="px-5 py-4 border-b border-gray-200">
            <h2 className="text-sm font-semibold text-gray-900">General Settings</h2>
          </div>
          <div className="p-5">
            <div className="max-w-xl space-y-4">
              <div>
                <label htmlFor="panel-name" className="mb-1.5 block text-sm font-medium text-gray-700">
                  Panel Name
                </label>
                <input
                  id="panel-name"
                  type="text"
                  className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus:outline-none"
                  defaultValue={brand.productName}
                />
              </div>
              <div>
                <label htmlFor="panel-url" className="mb-1.5 block text-sm font-medium text-gray-700">
                  Panel URL
                </label>
                <input
                  id="panel-url"
                  type="text"
                  className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus:outline-none"
                  defaultValue="https://panel.example.com"
                />
              </div>
              <div>
                <label htmlFor="panel-language" className="mb-1.5 block text-sm font-medium text-gray-700">
                  Default Language
                </label>
                <select
                  id="panel-language"
                  className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-brand-500 focus:ring-1 focus:ring-brand-500 focus:outline-none"
                  defaultValue="en"
                >
                  <option value="en">English</option>
                  <option value="vi">Tiếng Việt</option>
                </select>
              </div>
            </div>
          </div>
        </div>
      ) : null}

      {activeTab !== 'general' && activeTab !== 'panel-access' ? (
        <div className="rounded-lg border border-dashed border-gray-300 bg-white px-5 py-10 text-center">
          <p className="text-sm font-medium text-gray-900">Nothing here yet</p>
          <p className="mt-1 text-sm text-gray-600">
            This section has not been built out. Panel access settings live under the
            &quot;Panel access&quot; tab.
          </p>
        </div>
      ) : null}
    </div>
  );
}
