'use client';

/**
 * The strip of engines across the top of the Databases screen.
 *
 * aaPanel puts one tab per engine here and keeps the selection in the DOM. That
 * loses the selection on reload and gives an operator nothing to bookmark, so
 * this version keeps the selection in the URL instead - `?engine=postgresql` is
 * a link a DBA can send to a colleague.
 *
 * A tab whose engine the backend cannot manage still appears, and carries a
 * quiet marker saying so. Hiding it would leave an operator wondering whether
 * the panel forgot Redis or never had it.
 */

import { cn } from '@/lib/utils';
import {
  DATABASE_ENGINES,
  type DatabaseEngineId,
} from '@/types/databases';

export interface EngineTabsProps {
  active: DatabaseEngineId;
  onSelect: (id: DatabaseEngineId) => void;
  /** How many rows each tab currently holds, when the number is known. */
  counts?: Partial<Record<DatabaseEngineId, number | null>>;
}

export function EngineTabs({ active, onSelect, counts }: EngineTabsProps) {
  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white shadow-sm">
      <div role="tablist" aria-label="Database engines" className="flex min-w-max">
        {DATABASE_ENGINES.map((engine) => {
          const selected = engine.id === active;
          const count = counts?.[engine.id];
          return (
            <button
              key={engine.id}
              type="button"
              role="tab"
              id={`engine-tab-${engine.id}`}
              aria-selected={selected}
              aria-controls={`engine-panel-${engine.id}`}
              onClick={() => onSelect(engine.id)}
              className={cn(
                'relative flex items-center gap-2 whitespace-nowrap border-b-2 px-5 py-3 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500',
                selected
                  ? 'border-brand-600 bg-brand-50 text-brand-700'
                  : 'border-transparent text-gray-600 hover:bg-gray-50 hover:text-gray-900'
              )}
            >
              <span>{engine.label}</span>
              {typeof count === 'number' && (
                <span
                  className={cn(
                    'rounded-md px-1.5 py-0.5 text-xs font-semibold',
                    selected ? 'bg-brand-100 text-brand-700' : 'bg-gray-100 text-gray-600'
                  )}
                >
                  {count}
                </span>
              )}
              {engine.support !== 'managed' && (
                <span
                  title="The panel can register an instance of this engine but cannot create databases on it yet."
                  className="rounded-md border border-amber-200 bg-amber-50 px-1.5 py-0.5 text-[11px] font-medium text-amber-700"
                >
                  Limited
                </span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}

export default EngineTabs;
