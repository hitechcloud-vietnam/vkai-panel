'use client';

/**
 * The strip of sections across the top of the Security screen.
 *
 * aaPanel puts one tab per area here and keeps the selection in the DOM, which
 * loses it on reload and gives an operator nothing to send to a colleague. This
 * version keeps the selection in the URL instead: `/security?tab=firewall` is a
 * link, a bookmark, and survives a refresh.
 *
 * The selected tab carries the brand colour and nothing else does.
 */

import { cn } from '@/lib/utils';

import { SECURITY_TABS, type SecurityTabId } from './types';

export interface SecurityTabsProps {
  active: SecurityTabId;
  onSelect: (id: SecurityTabId) => void;
  /** A count to show on a tab, when the tab has a meaningful one. */
  counts?: Partial<Record<SecurityTabId, number | null>>;
  /** Tabs whose backend does not exist yet, marked so nobody hunts for it. */
  unavailable?: SecurityTabId[];
}

export function SecurityTabs({ active, onSelect, counts, unavailable = [] }: SecurityTabsProps) {
  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white shadow-sm">
      <div role="tablist" aria-label="Security sections" className="flex min-w-max">
        {SECURITY_TABS.map((tab) => {
          const selected = tab.id === active;
          const count = counts?.[tab.id];
          const notBuilt = unavailable.includes(tab.id);
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              id={`security-tab-${tab.id}`}
              aria-selected={selected}
              aria-controls={`security-panel-${tab.id}`}
              onClick={() => onSelect(tab.id)}
              className={cn(
                'relative flex items-center gap-2 whitespace-nowrap border-b-2 px-4 py-3 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500',
                selected
                  ? 'border-brand-600 bg-brand-50 text-brand-700'
                  : 'border-transparent text-gray-600 hover:bg-gray-50 hover:text-gray-900'
              )}
            >
              <span>{tab.label}</span>
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
              {notBuilt && (
                <span
                  title="The panel has no endpoint behind this section yet. It shows what is missing rather than a control that would do nothing."
                  className="rounded-md border border-amber-200 bg-amber-50 px-1.5 py-0.5 text-[11px] font-medium text-amber-700"
                >
                  No backend
                </span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}

export default SecurityTabs;
