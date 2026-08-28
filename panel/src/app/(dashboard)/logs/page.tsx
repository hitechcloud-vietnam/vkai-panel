/**
 * /logs - the Logs area.
 *
 * The tab lives in the query string, which means useSearchParams, which in the
 * app router must sit under a Suspense boundary or the whole route opts out of
 * static rendering and the build warns. The fallback is the page's own skeleton
 * rather than a spinner, so the header and the tab strip do not jump into place.
 */

import { Suspense } from 'react';
import LogsPageClient from '@/components/logs/LogsPageClient';

export const metadata = {
  title: 'Logs',
};

export default function LogsPage() {
  return (
    <Suspense fallback={<LogsSkeleton />}>
      <LogsPageClient />
    </Suspense>
  );
}

function LogsSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true">
      <span className="sr-only">Loading logs</span>
      <div className="h-6 w-32 animate-pulse rounded bg-gray-200" />
      <div className="h-4 w-96 max-w-full animate-pulse rounded bg-gray-100" />
      <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
        <div className="flex gap-4">
          {Array.from({ length: 5 }).map((_, index) => (
            <div key={index} className="h-4 w-24 animate-pulse rounded bg-gray-100" />
          ))}
        </div>
      </div>
      <div className="space-y-3 rounded-lg border border-gray-200 bg-white p-5 shadow-sm">
        {Array.from({ length: 8 }).map((_, index) => (
          <div key={index} className="h-4 animate-pulse rounded bg-gray-100" />
        ))}
      </div>
    </div>
  );
}
