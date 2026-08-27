'use client';

import { useState } from 'react';
import { HardDrive, Plus, Download, Upload, Trash2 } from 'lucide-react';

export default function BackupsPage() {
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Backups</h1>
          <p className="mt-1 text-sm text-gray-600">Manage backups and restore points</p>
        </div>
        <button
          type="button"
          className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
        >
          <Plus size={16} />
          Create Backup
        </button>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
        <div className="border-b border-gray-200 px-5 py-4">
          <h2 className="text-sm font-semibold text-gray-900">Backup History</h2>
        </div>
        <div className="px-5 py-12 text-center">
          <HardDrive className="mx-auto text-gray-300" size={40} />
          <h3 className="mt-3 text-sm font-semibold text-gray-900">No backups</h3>
          <p className="mt-1 text-sm text-gray-600">Create your first backup to get started</p>
        </div>
      </div>
    </div>
  );
}
