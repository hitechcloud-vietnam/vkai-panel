'use client';

/**
 * The Logs page: one page for the area, a horizontal strip of tabs, and one
 * content pane per tab - the layout aaPanel operators already navigate by.
 *
 * The selected tab lives in the URL as ?tab=..., so a tab can be bookmarked,
 * linked to a colleague mid-incident, and survives a reload. router.replace is
 * used rather than push so that switching tabs does not fill the back button
 * with the same page five times.
 *
 * Only the active pane is mounted. Each pane fetches on mount, and mounting all
 * five would fire every log query on the server the moment anybody opened the
 * page, for four panes nobody is looking at.
 */

import { useCallback } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { cn } from '@/lib/utils';
import PanelLogsTab from './PanelLogsTab';
import WebsiteLogsTab from './WebsiteLogsTab';
import AuditTab from './AuditTab';
import SshLoginTab from './SshLoginTab';
import SoftLogsTab from './SoftLogsTab';

const TABS = [
  { id: 'panel', label: 'Panel Logs' },
  { id: 'website', label: 'Website Logs' },
  { id: 'audit', label: 'Logs Audit' },
  { id: 'ssh', label: 'SSH Login Logs' },
  { id: 'soft', label: 'Soft Logs' },
] as const;

type TabId = (typeof TABS)[number]['id'];

const DEFAULT_TAB: TabId = 'panel';

function isTabId(value: string | null): value is TabId {
  return TABS.some((tab) => tab.id === value);
}

export default function LogsPageClient() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const requested = searchParams.get('tab');
  const active: TabId = isTabId(requested) ? requested : DEFAULT_TAB;

  const select = useCallback(
    (tab: TabId) => {
      const params = new URLSearchParams(searchParams.toString());
      params.set('tab', tab);
      router.replace(`${pathname}?${params.toString()}`, { scroll: false });
    },
    [pathname, router, searchParams]
  );

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">Logs</h1>
        <p className="mt-1 text-sm text-gray-600">
          Panel activity, website request logs, the audit trail, sign-in history and the journals of
          the installed software.
        </p>
      </div>

      <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
        <nav className="flex flex-wrap gap-1 px-2" aria-label="Log sections">
          {TABS.map((tab) => {
            const selected = tab.id === active;
            return (
              <button
                key={tab.id}
                type="button"
                onClick={() => select(tab.id)}
                aria-current={selected ? 'page' : undefined}
                className={cn(
                  '-mb-px border-b-2 px-4 py-3 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500',
                  selected
                    ? 'border-brand-600 text-brand-700'
                    : 'border-transparent text-gray-500 hover:text-gray-900'
                )}
              >
                {tab.label}
              </button>
            );
          })}
        </nav>
      </div>

      {active === 'panel' ? <PanelLogsTab /> : null}
      {active === 'website' ? <WebsiteLogsTab /> : null}
      {active === 'audit' ? <AuditTab /> : null}
      {active === 'ssh' ? <SshLoginTab /> : null}
      {active === 'soft' ? <SoftLogsTab /> : null}
    </div>
  );
}
