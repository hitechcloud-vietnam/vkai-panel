'use client';

import { useState } from 'react';
import { Shield, Plus, Edit, Trash2 } from 'lucide-react';

export default function SecurityPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Security</h1>
          <p className="text-dark-400 mt-1">Manage security settings and SSH keys</p>
        </div>
        <button className="btn btn-primary">
          <Plus size={16} />
          Add SSH Key
        </button>
      </div>

      <div className="card">
        <div className="text-center py-12">
          <Shield className="mx-auto text-dark-600" size={48} />
          <h3 className="mt-4 text-lg font-medium text-dark-300">No security configurations</h3>
          <p className="mt-2 text-dark-500">Configure security settings to get started</p>
        </div>
      </div>
    </div>
  );
}
