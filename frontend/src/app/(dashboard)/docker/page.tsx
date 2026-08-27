'use client';

import { useState } from 'react';
import { Container, Plus, Play, Square, Trash2 } from 'lucide-react';

export default function DockerPage() {
  const [activeTab, setActiveTab] = useState('containers');

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Docker</h1>
          <p className="text-dark-400 mt-1">Manage Docker containers, images, and volumes</p>
        </div>
        <button className="btn btn-primary">
          <Plus size={16} />
          New Container
        </button>
      </div>

      {/* Tabs */}
      <div className="tabs">
        <button
          className={`tab ${activeTab === 'containers' ? 'active' : ''}`}
          onClick={() => setActiveTab('containers')}
        >
          Containers
        </button>
        <button
          className={`tab ${activeTab === 'images' ? 'active' : ''}`}
          onClick={() => setActiveTab('images')}
        >
          Images
        </button>
        <button
          className={`tab ${activeTab === 'volumes' ? 'active' : ''}`}
          onClick={() => setActiveTab('volumes')}
        >
          Volumes
        </button>
        <button
          className={`tab ${activeTab === 'networks' ? 'active' : ''}`}
          onClick={() => setActiveTab('networks')}
        >
          Networks
        </button>
      </div>

      <div className="card">
        <div className="text-center py-12">
          <Container className="mx-auto text-dark-600" size={48} />
          <h3 className="mt-4 text-lg font-medium text-dark-300">No {activeTab}</h3>
          <p className="mt-2 text-dark-500">Create your first {activeTab.slice(0, -1)} to get started</p>
        </div>
      </div>
    </div>
  );
}
