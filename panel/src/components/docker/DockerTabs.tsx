'use client';

/**
 * The strip of areas across the top of the Docker screen.
 *
 * aaPanel puts one tab per area here and keeps the selection in the DOM, which
 * loses it on reload and leaves an operator nothing to bookmark. This version
 * keeps the selection in the URL, so `/docker?tab=volume` is a link one
 * engineer can send another and a reload lands where it left off.
 *
 * A tab the backend cannot serve still appears. Hiding it would leave an
 * operator wondering whether the panel forgot Docker Compose or never had it;
 * the quiet marker says which.
 */

import { cn } from '@/lib/utils';
import { DOCKER_TABS, type DockerTabId } from '@/types/docker';

export interface DockerTabsProps {
  active: DockerTabId;
  onSelect: (id: DockerTabId) => void;
  /** Tabs whose backend is entirely absent, marked so nobody hunts for a bug. */
  unavailable?: DockerTabId[];
}

export function DockerTabs({ active, onSelect, unavailable = [] }: DockerTabsProps) {
  const missing = new Set(unavailable);
  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white shadow-sm">
      <div role="tablist" aria-label="Docker areas" className="flex min-w-max">
        {DOCKER_TABS.map((tab) => {
          const selected = tab.id === active;
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              id={`docker-tab-${tab.id}`}
              aria-selected={selected}
              aria-controls={`docker-panel-${tab.id}`}
              onClick={() => onSelect(tab.id)}
              className={cn(
                'relative flex items-center gap-2 whitespace-nowrap border-b-2 px-5 py-3 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500',
                selected
                  ? 'border-brand-600 bg-brand-50 text-brand-700'
                  : 'border-transparent text-gray-600 hover:bg-gray-50 hover:text-gray-900'
              )}
            >
              <span>{tab.label}</span>
              {missing.has(tab.id) && (
                <span
                  title="This area has no working API behind it yet. The pane names exactly what is missing."
                  className="rounded-md border border-amber-200 bg-amber-50 px-1.5 py-0.5 text-[11px] font-medium text-amber-700"
                >
                  No API
                </span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}

export default DockerTabs;
