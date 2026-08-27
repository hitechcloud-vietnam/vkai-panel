'use client';

import { useState } from 'react';
import { Database, Plus, Search } from 'lucide-react';

export default function DatabasesPage() {
  const [activeTab, setActiveTab] = useState('mysql');

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Databases</h1>
          <p className="text-dark-400 mt-1">Manage MySQL, PostgreSQL, and Redis databases</p>
        </div>
        <button className="btn btn-primary">
          <Plus size={16} />
          Create Database
        </button>
      </div>

      {/* Tabs */}
      <div className="tabs">
        <button
          className={`tab ${activeTab === 'mysql' ? 'active' : ''}`}
          onClick={() => setActiveTab('mysql')}
        >
          MySQL / MariaDB
        </button>
        <button
          className={`tab ${activeTab === 'postgresql' ? 'active' : ''}`}
          onClick={() => setActiveTab('postgresql')}
        >
          PostgreSQL
        </button>
        <button
          className={`tab ${activeTab === 'redis' ? 'active' : ''}`}
          onClick={() => setActiveTab('redis')}
        >
          Redis
        </button>
      </div>

      {/* Content */}
      <div className="card">
        <div className="text-center py-12">
          <Database className="mx-auto text-dark-600" size={48} />
          <h3 className="mt-4 text-lg font-medium text-dark-300">
            No {activeTab === 'mysql' ? 'MySQL' : activeTab === 'postgresql' ? 'PostgreSQL' : 'Redis'} databases
          </h3>
          <p className="mt-2 text-dark-500">Create your first database to get started</p>
        </div>
      </div>
    </div>
  );
}
