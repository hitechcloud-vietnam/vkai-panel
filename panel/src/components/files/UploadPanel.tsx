'use client';

/**
 * What the uploads are doing, while they do it.
 *
 * Uploads run against the shared axios client with timeout: 0 and their own
 * AbortController, so a 90 MB file neither trips the panel's 15 second request
 * timeout nor blocks the listing: the operator can keep browsing, and can stop
 * a transfer that was started by mistake.
 */

import { CheckCircle2, X, XCircle } from 'lucide-react';

import type { UploadTask } from '@/types/files';
import { formatBytes } from './format';

export interface UploadPanelProps {
  tasks: UploadTask[];
  onCancel: (id: string) => void;
  onDismissFinished: () => void;
}

export default function UploadPanel({ tasks, onCancel, onDismissFinished }: UploadPanelProps) {
  if (tasks.length === 0) return null;
  const active = tasks.filter((task) => task.status === 'uploading').length;
  const finished = tasks.length - active;

  return (
    <section className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-gray-200 px-5 py-3">
        <h2 className="text-sm font-semibold text-gray-900">
          Uploads {active > 0 ? `(${active} in progress)` : ''}
        </h2>
        {finished > 0 ? (
          <button
            type="button"
            onClick={onDismissFinished}
            className="text-sm text-gray-600 hover:text-gray-900 focus:outline-none focus:ring-2 focus:ring-brand-500"
          >
            Clear finished
          </button>
        ) : null}
      </div>
      <ul className="divide-y divide-gray-100">
        {tasks.map((task) => (
          <li key={task.id} className="px-5 py-3">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-gray-900" title={task.destination}>
                  {task.name}
                </p>
                <p className="mt-0.5 truncate font-mono text-xs text-gray-500" title={task.destination}>
                  {task.destination}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <span className="text-xs text-gray-600">{formatBytes(task.size)}</span>
                {task.status === 'uploading' ? (
                  <button
                    type="button"
                    onClick={() => onCancel(task.id)}
                    aria-label={`Cancel upload of ${task.name}`}
                    className="rounded-md border border-gray-300 bg-white p-1.5 text-gray-600 hover:bg-gray-50 hover:text-gray-900 focus:outline-none focus:ring-2 focus:ring-brand-500"
                  >
                    <X size={14} aria-hidden="true" />
                  </button>
                ) : task.status === 'done' ? (
                  <CheckCircle2 size={16} className="text-emerald-600" aria-hidden="true" />
                ) : (
                  <XCircle size={16} className="text-red-600" aria-hidden="true" />
                )}
              </div>
            </div>

            {task.status === 'uploading' ? (
              <div className="mt-2">
                <div
                  className="h-1.5 w-full overflow-hidden rounded-md bg-gray-200"
                  role="progressbar"
                  aria-valuenow={task.progress ?? undefined}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-label={`Upload progress for ${task.name}`}
                >
                  <div
                    className="h-full bg-brand-600"
                    style={{ width: `${task.progress ?? 0}%` }}
                  />
                </div>
                <p className="mt-1 text-xs text-gray-500">
                  {task.progress === null
                    ? 'Sending — the browser did not report a total size for this file.'
                    : `${task.progress}%`}
                </p>
              </div>
            ) : null}

            {task.status === 'cancelled' ? (
              <p className="mt-1 text-xs text-gray-600">
                Cancelled. Anything already written to that path may be a partial file — check it
                before relying on it.
              </p>
            ) : null}

            {task.status === 'error' ? (
              <p className="mt-1 text-xs text-red-700">{task.error}</p>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  );
}
