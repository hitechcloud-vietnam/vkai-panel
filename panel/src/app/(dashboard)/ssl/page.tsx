'use client';

import { Shield, Plus, RefreshCw } from 'lucide-react';

export default function SSLPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">SSL Certificates</h1>
          <p className="mt-1 text-sm text-gray-600">Manage SSL/TLS certificates</p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-1"
          >
            <RefreshCw size={16} />
            Renew All
          </button>
          <button
            type="button"
            className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-1"
          >
            <Plus size={16} />
            Add Certificate
          </button>
        </div>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
        <div className="px-5 py-4 border-b border-gray-200">
          <h2 className="text-sm font-semibold text-gray-900">Certificates</h2>
        </div>
        <div className="py-12 text-center">
          <Shield className="mx-auto text-gray-400" size={40} />
          <h3 className="mt-4 text-sm font-semibold text-gray-900">No SSL certificates</h3>
          <p className="mt-1 text-sm text-gray-600">Add your first SSL certificate to get started</p>
        </div>
      </div>
    </div>
  );
}
