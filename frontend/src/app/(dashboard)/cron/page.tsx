'use client';

import { useState } from 'react';
import { Clock, Plus, Play, Pause, Trash2 } from 'lucide-react';

export default function CronPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-dark-50">Cron Jobs</h1>
          <p className="text-dark-400 mt-1">Manage scheduled tasks</p>
        </div>
        <button className="btn btn-primary">
          <Plus size={16} />
          Add Cron Job
        </button>
      </div>

      <div className="card">
        <div className="text-center py-12">
          <Clock className="mx-auto text-dark-600" size={48} />
          <h3 className="mt-4 text-lg font-medium text-dark-300">No cron jobs</h3>
          <p className="mt-2 text-dark-500">Add your first cron job to get started</p>
        </div>
      </div>
    </div>
  );
}
