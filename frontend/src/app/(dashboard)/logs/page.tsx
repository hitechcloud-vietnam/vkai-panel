'use client';

import { useState } from 'react';
import { Activity, RefreshCw } from 'lucide-react';

export default function LogsPage() {
  const [activeTab, setActiveTab] = useState('audit');

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Logs</h1>
          <p className="text-dark-400 mt-1">View audit logs and system logs</p>
        </div>
        <button className="btn btn-secondary">
          <RefreshCw size={16} />
          Refresh
        </button>
      </div>

      {/* Tabs */}
      <div className="tabs">
        <button
          className={`tab ${activeTab === 'audit' ? 'active' : ''}`}
          onClick={() => setActiveTab('audit')}
        >
          Audit Logs
        </button>
        <button
          className={`tab ${activeTab === 'system' ? 'active' : ''}`}
          onClick={() => setActiveTab('system')}
        >
          System Logs
        </button>
        <button
          className={`tab ${activeTab === 'access' ? 'active' : ''}`}
          onClick={() => setActiveTab('access')}
        >
          Access Logs
        </button>
      </div>

      <div className="card">
        <div className="text-center py-12">
          <Activity className="mx-auto text-dark-600" size={48} />
          <h3 className="mt-4 text-lg font-medium text-dark-300">No logs</h3>
          <p className="mt-2 text-dark-500">Logs will appear here as actions are performed</p>
        </div>
      </div>
    </div>
  );
}
