'use client';

import { useState } from 'react';
import { Settings, Save } from 'lucide-react';

export default function SettingsPage() {
  const [activeTab, setActiveTab] = useState('general');

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Settings</h1>
          <p className="text-dark-400 mt-1">Configure panel settings</p>
        </div>
        <button className="btn btn-primary">
          <Save size={16} />
          Save Changes
        </button>
      </div>

      {/* Tabs */}
      <div className="tabs">
        <button
          className={`tab ${activeTab === 'general' ? 'active' : ''}`}
          onClick={() => setActiveTab('general')}
        >
          General
        </button>
        <button
          className={`tab ${activeTab === 'security' ? 'active' : ''}`}
          onClick={() => setActiveTab('security')}
        >
          Security
        </button>
        <button
          className={`tab ${activeTab === 'notifications' ? 'active' : ''}`}
          onClick={() => setActiveTab('notifications')}
        >
          Notifications
        </button>
        <button
          className={`tab ${activeTab === 'backup' ? 'active' : ''}`}
          onClick={() => setActiveTab('backup')}
        >
          Backup
        </button>
      </div>

      <div className="card">
        <div className="space-y-6">
          <div>
            <h3 className="text-lg font-medium text-dark-100 mb-4">General Settings</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-2">
                  Panel Name
                </label>
                <input type="text" className="input" defaultValue="vKAI Panel" />
              </div>
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-2">
                  Panel URL
                </label>
                <input type="text" className="input" defaultValue="https://panel.example.com" />
              </div>
              <div>
                <label className="block text-sm font-medium text-dark-300 mb-2">
                  Default Language
                </label>
                <select className="input">
                  <option value="en">English</option>
                  <option value="vi">Tiếng Việt</option>
                </select>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
