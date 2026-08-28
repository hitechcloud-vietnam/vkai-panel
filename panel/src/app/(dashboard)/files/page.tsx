'use client';

/**
 * /files - the file manager.
 *
 * The current directory is held in the URL as ?path=, which is what makes a
 * directory bookmarkable and a reload land where the operator was rather than
 * back at the root. Reading a search parameter opts the route into client-side
 * rendering, so the manager sits behind a Suspense boundary; without one the
 * production build fails on this page.
 */

import { Suspense } from 'react';
import { RefreshCw } from 'lucide-react';

import FileManager from '@/components/files/FileManager';

function Loading() {
  return (
    <div className="flex h-64 items-center justify-center">
      <RefreshCw className="h-6 w-6 animate-spin text-gray-400" aria-hidden="true" />
      <span className="ml-2 text-sm text-gray-600">Opening the file manager...</span>
    </div>
  );
}

export default function FilesPage() {
  return (
    <Suspense fallback={<Loading />}>
      <FileManager />
    </Suspense>
  );
}
