'use client';

import { useState } from 'react';
import { Flame, Plus, Trash2 } from 'lucide-react';

export default function FirewallPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Firewall</h1>
          <p className="text-dark-400 mt-1">Manage firewall rules and security</p>
        </div>
        <button className="btn btn-primary">
          <Plus size={16} />
          Add Rule
        </button>
      </div>

      <div className="card">
        <div className="text-center py-12">
          <Flame className="mx-auto text-dark-600" size={48} />
          <h3 className="mt-4 text-lg font-medium text-dark-300">No firewall rules</h3>
          <p className="mt-2 text-dark-500">Add your first firewall rule to get started</p>
        </div>
      </div>
    </div>
  );
}
