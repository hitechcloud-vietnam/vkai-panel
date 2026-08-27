'use client';

import { useState } from 'react';
import { Key, Plus, Copy, Trash2, Eye, EyeOff } from 'lucide-react';

export default function APIKeysPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">API Keys</h1>
          <p className="text-dark-400 mt-1">Manage API keys for programmatic access</p>
        </div>
        <button className="btn btn-primary">
          <Plus size={16} />
          Generate API Key
        </button>
      </div>

      <div className="card">
        <div className="text-center py-12">
          <Key className="mx-auto text-dark-600" size={48} />
          <h3 className="mt-4 text-lg font-medium text-dark-300">No API keys</h3>
          <p className="mt-2 text-dark-500">Generate your first API key to get started</p>
        </div>
      </div>
    </div>
  );
}
