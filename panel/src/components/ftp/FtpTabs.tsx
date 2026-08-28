'use client';

/**
 * The strip of panes across the top of the FTP screen.
 *
 * aaPanel keeps the selected tab in the DOM, which loses it on reload and gives
 * an operator nothing to bookmark. Here the selection lives in the query string
 * - `/ftp?tab=service` is a link one engineer can send another when a customer
 * cannot connect through a firewall.
 */

import { cn } from '@/lib/utils';
import { FTP_TABS, type FtpTabId } from './types';

export function FtpTabs({
  active,
  onSelect,
}: {
  active: FtpTabId;
  onSelect: (id: FtpTabId) => void;
}) {
  return (
    <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white shadow-sm">
      <div role="tablist" aria-label="FTP sections" className="flex min-w-max">
        {FTP_TABS.map((tab) => {
          const selected = tab.id === active;
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              id={`ftp-tab-${tab.id}`}
              aria-selected={selected}
              aria-controls={`ftp-panel-${tab.id}`}
              onClick={() => onSelect(tab.id)}
              className={cn(
                'whitespace-nowrap border-b-2 px-5 py-3 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500',
                selected
                  ? 'border-brand-600 bg-brand-50 text-brand-700'
                  : 'border-transparent text-gray-600 hover:bg-gray-50 hover:text-gray-900'
              )}
            >
              {tab.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}

export default FtpTabs;
