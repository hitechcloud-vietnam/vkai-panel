'use client';

/**
 * The two strips of navigation this section uses.
 *
 * aaPanel puts one horizontal strip of tabs above one content pane per type and
 * keeps the selection in the DOM. That loses the selection on reload and gives
 * an operator nothing to bookmark, so here the selection lives in the URL:
 * `?tab=mail-marketing&sub=suspend-list` is a link that can be sent to someone.
 */

import { cn } from '@/lib/utils';

export interface TabItem {
  id: string;
  label: string;
}

/** The top strip: one entry per area of the mail system. */
export function TopTabs({
  items,
  active,
  onSelect,
}: {
  items: readonly TabItem[];
  active: string;
  onSelect: (id: string) => void;
}) {
  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white shadow-sm">
      <div role="tablist" aria-label="Email server areas" className="flex min-w-max">
        {items.map((item) => {
          const selected = item.id === active;
          return (
            <button
              key={item.id}
              type="button"
              role="tab"
              id={`email-tab-${item.id}`}
              aria-selected={selected}
              aria-controls={`email-panel-${item.id}`}
              onClick={() => onSelect(item.id)}
              className={cn(
                'whitespace-nowrap border-b-2 px-5 py-3 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500',
                selected
                  ? 'border-brand-600 bg-brand-50 text-brand-700'
                  : 'border-transparent text-gray-600 hover:bg-gray-50 hover:text-gray-900'
              )}
            >
              {item.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}

/** The quieter second strip, inside one area. */
export function SubNav({
  items,
  active,
  onSelect,
  label,
}: {
  items: readonly TabItem[];
  active: string;
  onSelect: (id: string) => void;
  label: string;
}) {
  return (
    <div className="overflow-x-auto">
      <div role="tablist" aria-label={label} className="flex min-w-max gap-1">
        {items.map((item) => {
          const selected = item.id === active;
          return (
            <button
              key={item.id}
              type="button"
              role="tab"
              aria-selected={selected}
              onClick={() => onSelect(item.id)}
              className={cn(
                'whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500',
                selected
                  ? 'bg-brand-600 text-white'
                  : 'bg-white text-gray-600 hover:bg-gray-100 hover:text-gray-900'
              )}
            >
              {item.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}
