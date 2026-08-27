'use client';

import { useState } from 'react';
import { Shield, Plus, RefreshCw } from 'lucide-react';

export default function SSLPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">SSL Certificates</h1>
          <p className="text-dark-400 mt-1">Manage SSL/TLS certificates</p>
        </div>
        <div className="flex items-center gap-3">
          <button className="btn btn-secondary">
            <RefreshCw size={16} />
            Renew All
          </button>
          <button className="btn btn-primary">
            <Plus size={16} />
            Add Certificate
          </button>
        </div>
      </div>

      <div className="card">
        <div className="text-center py-12">
          <Shield className="mx-auto text-dark-600" size={48} />
          <h3 className="mt-4 text-lg font-medium text-dark-300">No SSL certificates</h3>
          <p className="mt-2 text-dark-500">Add your first SSL certificate to get started</p>
        </div>
      </div>
    </div>
  );
}
