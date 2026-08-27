'use client';

import { useState } from 'react';
import { GitBranch, Plus, RefreshCw } from 'lucide-react';

export default function DeploymentsPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Deployments</h1>
          <p className="text-dark-400 mt-1">Manage Git deployments and CI/CD</p>
        </div>
        <button className="btn btn-primary">
          <Plus size={16} />
          New Deployment
        </button>
      </div>

      <div className="card">
        <div className="text-center py-12">
          <GitBranch className="mx-auto text-dark-600" size={48} />
          <h3 className="mt-4 text-lg font-medium text-dark-300">No deployments</h3>
          <p className="mt-2 text-dark-500">Set up your first deployment to get started</p>
        </div>
      </div>
    </div>
  );
}
